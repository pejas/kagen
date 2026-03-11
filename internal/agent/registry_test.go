package agent

import (
	"errors"
	"testing"

	kagerr "github.com/pejas/kagen/internal/errors"
)

func TestRegistryGetKnownTypes(t *testing.T) {
	t.Parallel()

	reg := NewRegistry(nil, "")
	for _, tt := range []Type{Claude, Codex, OpenCode} {
		a, err := reg.Get(tt)
		if err != nil {
			t.Errorf("Get(%q) returned error: %v", tt, err)
			continue
		}
		if a.AgentType() != tt {
			t.Errorf("expected AgentType=%q, got %q", tt, a.AgentType())
		}
	}
}

func TestRegistryGetUnknownType(t *testing.T) {
	t.Parallel()

	reg := NewRegistry(nil, "")
	_, err := reg.Get(Type("unknown"))
	if !errors.Is(err, kagerr.ErrAgentUnknown) {
		t.Errorf("expected ErrAgentUnknown, got %v", err)
	}
}

func TestRegistryAvailable(t *testing.T) {
	t.Parallel()

	reg := NewRegistry(nil, "")
	available := reg.Available()
	if len(available) != 3 {
		t.Fatalf("expected 3 available agents, got %d", len(available))
	}

	expected := map[Type]bool{Claude: true, Codex: true, OpenCode: true}
	for _, a := range available {
		if !expected[a] {
			t.Errorf("unexpected agent type %q in Available()", a)
		}
	}
}

func TestTypeFromString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   string
		want    Type
		wantErr bool
	}{
		{"claude", Claude, false},
		{"codex", Codex, false},
		{"opencode", OpenCode, false},
		{"unknown", "", true},
		{"", "", true},
	}

	for _, tc := range tests {
		got, err := TypeFromString(tc.input)
		if tc.wantErr {
			if !errors.Is(err, kagerr.ErrAgentUnknown) {
				t.Errorf("TypeFromString(%q): expected ErrAgentUnknown, got %v", tc.input, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("TypeFromString(%q) returned unexpected error: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("TypeFromString(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
