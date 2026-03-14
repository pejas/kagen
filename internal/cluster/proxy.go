package cluster

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/proxy"
	"github.com/pejas/kagen/internal/ui"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	proxyDeploymentName = "egress-proxy"
	proxyServiceName    = "egress-proxy"
	proxyPolicyName     = "egress-proxy-egress"
	proxyConfigMapName  = "egress-proxy-config"
	proxyPort           = 8888
	proxyConfigDir      = "/etc/kagen-proxy"
	proxyConfigChecksum = "kagen.io/proxy-config-sha256"
)

// EnsureProxy reconciles the repo-scoped proxy workload, service, and egress policy.
func (k *KubeManager) EnsureProxy(ctx context.Context, repo *git.Repository, policy *proxy.Policy, imageRef string) error {
	nsName := fmt.Sprintf("kagen-%s", repo.ID())
	if policy == nil || len(policy.AllowedDestinations) == 0 {
		ui.Verbose("Deleting proxy resources in %s because no allowlist is configured", nsName)
		return k.deleteProxyResources(ctx, nsName)
	}
	ui.Verbose("Reconciling proxy resources in %s for %d destination(s)", nsName, len(policy.AllowedDestinations))

	checksum, err := k.ensureProxyConfig(ctx, nsName, repo.ID(), policy)
	if err != nil {
		return err
	}
	if err := k.ensureProxyService(ctx, nsName, repo.ID()); err != nil {
		return err
	}
	if err := k.ensureProxyDeployment(ctx, nsName, repo.ID(), checksum, imageRef); err != nil {
		return err
	}
	if err := k.waitForProxyReady(ctx, nsName); err != nil {
		return err
	}

	return nil
}

// EnsureProxyPolicy reconciles the repo-scoped agent egress policy once the
// workspace bootstrap path is complete and proxy enforcement should become
// active for the agent pod.
func (k *KubeManager) EnsureProxyPolicy(ctx context.Context, repo *git.Repository) error {
	nsName := fmt.Sprintf("kagen-%s", repo.ID())
	return k.ensureAgentNetworkPolicy(ctx, nsName, repo.ID())
}

func (k *KubeManager) deleteProxyResources(ctx context.Context, namespace string) error {
	if err := k.client.NetworkingV1().NetworkPolicies(namespace).Delete(ctx, proxyPolicyName, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("deleting proxy networkpolicy: %w", err)
	}
	if err := k.client.CoreV1().Services(namespace).Delete(ctx, proxyServiceName, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("deleting proxy service: %w", err)
	}
	if err := k.client.AppsV1().Deployments(namespace).Delete(ctx, proxyDeploymentName, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("deleting proxy deployment: %w", err)
	}
	if err := k.client.CoreV1().ConfigMaps(namespace).Delete(ctx, proxyConfigMapName, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("deleting proxy configmap: %w", err)
	}

	return nil
}

// ProxyReady reports whether the repo-scoped proxy deployment is ready.
func (k *KubeManager) ProxyReady(ctx context.Context, repo *git.Repository) (bool, error) {
	nsName := fmt.Sprintf("kagen-%s", repo.ID())
	deployment, err := k.client.AppsV1().Deployments(nsName).Get(ctx, proxyDeploymentName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("getting proxy deployment: %w", err)
	}

	return deployment.Status.ReadyReplicas > 0, nil
}

func (k *KubeManager) ensureProxyConfig(ctx context.Context, namespace, repoID string, policy *proxy.Policy) (string, error) {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      proxyConfigMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": "kagen-proxy",
				"kagen.io/repo-id":       repoID,
			},
		},
		Data: map[string]string{
			"allowlist":      proxyAllowlist(policy.AllowedDestinations),
			"tinyproxy.conf": tinyproxyConfig(),
		},
	}
	checksum := proxyConfigDataChecksum(configMap.Data)

	_, err := k.client.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
	if err == nil {
		return checksum, nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return "", fmt.Errorf("creating proxy configmap: %w", err)
	}

	current, err := k.client.CoreV1().ConfigMaps(namespace).Get(ctx, proxyConfigMapName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("getting proxy configmap: %w", err)
	}

	current.Data = configMap.Data
	current.Labels = configMap.Labels
	if _, err := k.client.CoreV1().ConfigMaps(namespace).Update(ctx, current, metav1.UpdateOptions{}); err != nil {
		return "", fmt.Errorf("updating proxy configmap: %w", err)
	}

	return checksum, nil
}

func proxyConfigDataChecksum(data map[string]string) string {
	sum := sha256.Sum256([]byte(data["allowlist"] + "\n---\n" + data["tinyproxy.conf"]))
	return hex.EncodeToString(sum[:])
}

func tinyproxyConfig() string {
	return fmt.Sprintf(`Port %d
Timeout 600
LogLevel Info
LogFile "/dev/stdout"
PidFile "/tmp/tinyproxy.pid"
MaxClients 100
StartServers 2
MinSpareServers 1
MaxSpareServers 5
DisableViaHeader Yes
ConnectPort 80
ConnectPort 443
ConnectPort 22
Filter "%s/allowlist"
FilterDefaultDeny Yes
`, proxyPort, proxyConfigDir)
}

func proxyAllowlist(destinations []string) string {
	patterns := make([]string, 0, len(destinations))
	for _, destination := range destinations {
		host := strings.TrimSpace(destination)
		if host == "" {
			continue
		}

		patterns = append(patterns, regexp.QuoteMeta(host))
	}

	return strings.Join(patterns, "\n")
}

func (k *KubeManager) ensureProxyDeployment(ctx context.Context, namespace, repoID, checksum, imageRef string) error {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      proxyDeploymentName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": "kagen-proxy",
				"kagen.io/repo-id":       repoID,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": "kagen-proxy",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name": "kagen-proxy",
						"kagen.io/repo-id":       repoID,
					},
					Annotations: map[string]string{
						proxyConfigChecksum: checksum,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						proxyContainer(imageRef),
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: proxyConfigMapName,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := k.client.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err == nil {
		return nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("creating proxy deployment: %w", err)
	}

	current, err := k.client.AppsV1().Deployments(namespace).Get(ctx, proxyDeploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting proxy deployment: %w", err)
	}

	current.Labels = deployment.Labels
	current.Spec.Template.Labels = deployment.Spec.Template.Labels
	current.Spec.Template.Annotations = deployment.Spec.Template.Annotations
	current.Spec.Template.Spec = deployment.Spec.Template.Spec
	if _, err := k.client.AppsV1().Deployments(namespace).Update(ctx, current, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating proxy deployment: %w", err)
	}

	return nil
}

func proxyContainer(imageRef string) corev1.Container {
	return corev1.Container{
		Name:    "proxy",
		Image:   imageRef,
		Command: []string{"/bin/sh", "-lc"},
		Args:    []string{"exec tinyproxy -d -c " + proxyConfigDir + `/tinyproxy.conf`},
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: proxyPort,
			},
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(proxyPort),
				},
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "config",
				MountPath: proxyConfigDir,
				ReadOnly:  true,
			},
		},
	}
}

func (k *KubeManager) ensureProxyService(ctx context.Context, namespace, repoID string) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      proxyServiceName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": "kagen-proxy",
				"kagen.io/repo-id":       repoID,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app.kubernetes.io/name": "kagen-proxy",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       proxyPort,
					TargetPort: intstr.FromInt(proxyPort),
				},
			},
		},
	}

	_, err := k.client.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err == nil {
		return nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("creating proxy service: %w", err)
	}

	current, err := k.client.CoreV1().Services(namespace).Get(ctx, proxyServiceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting proxy service: %w", err)
	}

	current.Labels = service.Labels
	current.Spec.Ports = service.Spec.Ports
	current.Spec.Selector = service.Spec.Selector
	if _, err := k.client.CoreV1().Services(namespace).Update(ctx, current, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating proxy service: %w", err)
	}

	return nil
}

func (k *KubeManager) ensureAgentNetworkPolicy(ctx context.Context, namespace, repoID string) error {
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      proxyPolicyName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": "kagen-proxy",
				"kagen.io/repo-id":       repoID,
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": "kagen-agent",
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					To: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app.kubernetes.io/name": "kagen-proxy",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: tcpProtocol(),
							Port:     intStrPtr(proxyPort),
						},
					},
				},
				{
					To: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "forgejo",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: tcpProtocol(),
							Port:     intStrPtr(3000),
						},
						{
							Protocol: tcpProtocol(),
							Port:     intStrPtr(22),
						},
					},
				},
				{
					To: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": "kube-system",
								},
							},
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"k8s-app": "kube-dns",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: udpProtocol(),
							Port:     intStrPtr(53),
						},
						{
							Protocol: tcpProtocol(),
							Port:     intStrPtr(53),
						},
					},
				},
			},
		},
	}

	_, err := k.client.NetworkingV1().NetworkPolicies(namespace).Create(ctx, policy, metav1.CreateOptions{})
	if err == nil {
		return nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("creating proxy networkpolicy: %w", err)
	}

	current, err := k.client.NetworkingV1().NetworkPolicies(namespace).Get(ctx, proxyPolicyName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting proxy networkpolicy: %w", err)
	}

	current.Labels = policy.Labels
	current.Spec = policy.Spec
	if _, err := k.client.NetworkingV1().NetworkPolicies(namespace).Update(ctx, current, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating proxy networkpolicy: %w", err)
	}

	return nil
}

func (k *KubeManager) waitForProxyReady(ctx context.Context, namespace string) error {
	deadline := time.Now().Add(2 * time.Minute)
	attempt := 0
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			attempt++
			deployment, err := k.client.AppsV1().Deployments(namespace).Get(ctx, proxyDeploymentName, metav1.GetOptions{})
			if err == nil && deployment.Status.ReadyReplicas > 0 {
				ui.Verbose("Proxy deployment %s/%s is ready after %d attempt(s)", namespace, proxyDeploymentName, attempt)
				return nil
			}
			if err != nil && !k8serrors.IsNotFound(err) {
				return fmt.Errorf("getting proxy deployment: %w", err)
			}
			if ui.VerboseEnabled() && (attempt == 1 || attempt%5 == 0) {
				ui.Verbose("Waiting for proxy deployment %s/%s readiness (attempt %d)", namespace, proxyDeploymentName, attempt)
			}
			if err := waitForRetry(ctx, 2*time.Second); err != nil {
				return err
			}
		}
	}

	return fmt.Errorf("timed out waiting for proxy deployment %s/%s to become ready", namespace, proxyDeploymentName)
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func int32Ptr(value int32) *int32 {
	return &value
}

func intStrPtr(value int) *intstr.IntOrString {
	intValue := intstr.FromInt(value)
	return &intValue
}

func tcpProtocol() *corev1.Protocol {
	protocol := corev1.ProtocolTCP
	return &protocol
}

func udpProtocol() *corev1.Protocol {
	protocol := corev1.ProtocolUDP
	return &protocol
}
