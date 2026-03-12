package cluster

import "testing"

func TestPortForwardSpecHonoursRequestedLocalPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		localPort  int
		remotePort int
		want       string
	}{
		{
			name:       "ephemeral local port",
			localPort:  0,
			remotePort: 3000,
			want:       ":3000",
		},
		{
			name:       "requested local port",
			localPort:  18080,
			remotePort: 3000,
			want:       "18080:3000",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := portForwardSpec(test.localPort, test.remotePort)
			if err != nil {
				t.Fatalf("portForwardSpec() returned error: %v", err)
			}
			if got != test.want {
				t.Fatalf("portForwardSpec() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestParseForwardingLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		line           string
		wantLocalPort  int
		wantRemotePort int
		wantErr        bool
	}{
		{
			name:           "ipv4",
			line:           "Forwarding from 127.0.0.1:54321 -> 3000",
			wantLocalPort:  54321,
			wantRemotePort: 3000,
		},
		{
			name:           "ipv6",
			line:           "Forwarding from [::1]:54321 -> 3000",
			wantLocalPort:  54321,
			wantRemotePort: 3000,
		},
		{
			name:    "non-forwarding line",
			line:    "Handling connection for 3000",
			wantErr: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			gotLocalPort, gotRemotePort, err := parseForwardingLine(test.line)
			if test.wantErr {
				if err == nil {
					t.Fatal("parseForwardingLine() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseForwardingLine() returned error: %v", err)
			}
			if gotLocalPort != test.wantLocalPort {
				t.Fatalf("local port = %d, want %d", gotLocalPort, test.wantLocalPort)
			}
			if gotRemotePort != test.wantRemotePort {
				t.Fatalf("remote port = %d, want %d", gotRemotePort, test.wantRemotePort)
			}
		})
	}
}
