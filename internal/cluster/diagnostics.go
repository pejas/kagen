package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// DiagnosticsInspector captures machine-readable runtime diagnostics from the cluster.
type DiagnosticsInspector struct {
	client kubernetes.Interface
}

// PodSnapshot records the useful, stable parts of a runtime pod state.
type PodSnapshot struct {
	Name                  string                   `json:"name"`
	Namespace             string                   `json:"namespace"`
	Phase                 corev1.PodPhase          `json:"phase"`
	PodIP                 string                   `json:"pod_ip,omitempty"`
	StartTime             *metav1.Time             `json:"start_time,omitempty"`
	InitContainerStatuses []corev1.ContainerStatus `json:"init_container_statuses,omitempty"`
	ContainerStatuses     []corev1.ContainerStatus `json:"container_statuses,omitempty"`
	Conditions            []corev1.PodCondition    `json:"conditions,omitempty"`
}

// EventSnapshot records a deterministic pod event view.
type EventSnapshot struct {
	Type                string           `json:"type,omitempty"`
	Reason              string           `json:"reason,omitempty"`
	Message             string           `json:"message,omitempty"`
	Action              string           `json:"action,omitempty"`
	Count               int32            `json:"count,omitempty"`
	FirstTimestamp      metav1.Time      `json:"first_timestamp,omitempty"`
	LastTimestamp       metav1.Time      `json:"last_timestamp,omitempty"`
	EventTime           metav1.MicroTime `json:"event_time,omitempty"`
	ReportingController string           `json:"reporting_controller,omitempty"`
}

// DeploymentSnapshot records a deterministic readiness view for a deployment.
type DeploymentSnapshot struct {
	Name                string                       `json:"name"`
	Namespace           string                       `json:"namespace"`
	Replicas            int32                        `json:"replicas,omitempty"`
	ReadyReplicas       int32                        `json:"ready_replicas,omitempty"`
	UpdatedReplicas     int32                        `json:"updated_replicas,omitempty"`
	UnavailableReplicas int32                        `json:"unavailable_replicas,omitempty"`
	Conditions          []appsv1.DeploymentCondition `json:"conditions,omitempty"`
}

// DiagnosticBundle groups the cluster artefacts captured for a failed session operation.
type DiagnosticBundle struct {
	PodStatus       *PodSnapshot        `json:"pod_status,omitempty"`
	PodEvents       []EventSnapshot     `json:"pod_events,omitempty"`
	ContainerLogs   map[string]string   `json:"container_logs,omitempty"`
	ProxyDeployment *DeploymentSnapshot `json:"proxy_deployment,omitempty"`
	CaptureErrors   map[string]string   `json:"capture_errors,omitempty"`
}

// DiagnosticsRequest describes the runtime objects to inspect.
type DiagnosticsRequest struct {
	Namespace      string
	PodName        string
	AgentContainer string
}

// NewDiagnosticsInspector constructs a cluster diagnostics inspector for the provided kube context.
func NewDiagnosticsInspector(kubeCtx string) (*DiagnosticsInspector, error) {
	client, err := NewClientset(kubeCtx)
	if err != nil {
		return nil, err
	}

	return NewDiagnosticsInspectorForClient(client), nil
}

// NewDiagnosticsInspectorForClient constructs an inspector from an existing client.
func NewDiagnosticsInspectorForClient(client kubernetes.Interface) *DiagnosticsInspector {
	return &DiagnosticsInspector{client: client}
}

// Collect captures pod, event, log, and proxy deployment diagnostics.
func (i *DiagnosticsInspector) Collect(ctx context.Context, req DiagnosticsRequest) DiagnosticBundle {
	bundle := DiagnosticBundle{
		ContainerLogs: map[string]string{},
		CaptureErrors: map[string]string{},
	}
	if i == nil || i.client == nil {
		bundle.CaptureErrors["cluster"] = "cluster diagnostics inspector is not configured"
		return bundle
	}
	if req.Namespace == "" {
		bundle.CaptureErrors["namespace"] = "namespace is required for cluster diagnostics"
		return bundle
	}

	if req.PodName != "" {
		pod, err := i.client.CoreV1().Pods(req.Namespace).Get(ctx, req.PodName, metav1.GetOptions{})
		if err != nil {
			bundle.CaptureErrors["pod_status"] = fmt.Sprintf("getting pod %s/%s: %v", req.Namespace, req.PodName, err)
		} else {
			bundle.PodStatus = snapshotPod(pod)
		}

		events, err := i.client.CoreV1().Events(req.Namespace).List(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("involvedObject.kind=Pod,involvedObject.name=%s", req.PodName),
		})
		if err != nil {
			bundle.CaptureErrors["pod_events"] = fmt.Sprintf("listing pod events for %s/%s: %v", req.Namespace, req.PodName, err)
		} else {
			bundle.PodEvents = snapshotEvents(events.Items)
		}

		for _, containerName := range logContainers(req.AgentContainer) {
			content, err := i.readContainerLog(ctx, req.Namespace, req.PodName, containerName)
			if err != nil {
				bundle.CaptureErrors["logs."+containerName] = err.Error()
				continue
			}
			bundle.ContainerLogs[containerName] = content
		}
	}

	deployment, err := i.client.AppsV1().Deployments(req.Namespace).Get(ctx, proxyDeploymentName, metav1.GetOptions{})
	if err != nil {
		bundle.CaptureErrors["proxy_deployment"] = fmt.Sprintf("getting proxy deployment %s/%s: %v", req.Namespace, proxyDeploymentName, err)
	} else {
		bundle.ProxyDeployment = snapshotDeployment(deployment)
	}

	if len(bundle.ContainerLogs) == 0 {
		bundle.ContainerLogs = nil
	}
	if len(bundle.CaptureErrors) == 0 {
		bundle.CaptureErrors = nil
	}

	return bundle
}

func (i *DiagnosticsInspector) readContainerLog(ctx context.Context, namespace, podName, containerName string) (string, error) {
	request := i.client.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container: containerName,
	})

	stream, err := request.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("reading logs for %s/%s container %s: %w", namespace, podName, containerName, err)
	}
	defer func() {
		_ = stream.Close()
	}()

	content, err := io.ReadAll(stream)
	if err != nil {
		return "", fmt.Errorf("copying logs for %s/%s container %s: %w", namespace, podName, containerName, err)
	}

	return string(content), nil
}

func snapshotPod(pod *corev1.Pod) *PodSnapshot {
	if pod == nil {
		return nil
	}

	return &PodSnapshot{
		Name:                  pod.Name,
		Namespace:             pod.Namespace,
		Phase:                 pod.Status.Phase,
		PodIP:                 pod.Status.PodIP,
		StartTime:             pod.Status.StartTime,
		InitContainerStatuses: cloneJSONSlice(pod.Status.InitContainerStatuses),
		ContainerStatuses:     cloneJSONSlice(pod.Status.ContainerStatuses),
		Conditions:            cloneJSONSlice(pod.Status.Conditions),
	}
}

func snapshotEvents(items []corev1.Event) []EventSnapshot {
	if len(items) == 0 {
		return nil
	}

	sort.Slice(items, func(i, j int) bool {
		left := eventSortKey(items[i])
		right := eventSortKey(items[j])
		return left < right
	})

	snapshots := make([]EventSnapshot, 0, len(items))
	for _, item := range items {
		snapshots = append(snapshots, EventSnapshot{
			Type:                item.Type,
			Reason:              item.Reason,
			Message:             item.Message,
			Action:              item.Action,
			Count:               item.Count,
			FirstTimestamp:      item.FirstTimestamp,
			LastTimestamp:       item.LastTimestamp,
			EventTime:           item.EventTime,
			ReportingController: item.ReportingController,
		})
	}

	return snapshots
}

func eventSortKey(event corev1.Event) string {
	return fmt.Sprintf(
		"%s|%s|%s|%s|%s",
		event.LastTimestamp.UTC().Format("2006-01-02T15:04:05.000000000Z07:00"),
		event.FirstTimestamp.UTC().Format("2006-01-02T15:04:05.000000000Z07:00"),
		event.Reason,
		event.Type,
		event.Message,
	)
}

func snapshotDeployment(deployment *appsv1.Deployment) *DeploymentSnapshot {
	if deployment == nil {
		return nil
	}

	replicas := int32(0)
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}

	return &DeploymentSnapshot{
		Name:                deployment.Name,
		Namespace:           deployment.Namespace,
		Replicas:            replicas,
		ReadyReplicas:       deployment.Status.ReadyReplicas,
		UpdatedReplicas:     deployment.Status.UpdatedReplicas,
		UnavailableReplicas: deployment.Status.UnavailableReplicas,
		Conditions:          cloneJSONSlice(deployment.Status.Conditions),
	}
}

func logContainers(agentContainer string) []string {
	containers := []string{"workspace-sync"}
	if agentContainer != "" {
		containers = append(containers, agentContainer)
	}

	return containers
}

func cloneJSONSlice[T any](input []T) []T {
	if len(input) == 0 {
		return nil
	}

	raw, err := json.Marshal(input)
	if err != nil {
		return append([]T(nil), input...)
	}

	var cloned []T
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return append([]T(nil), input...)
	}

	return cloned
}
