//go:build windows && desktop

package workspacehost

import (
	"path/filepath"
	"testing"
)

func TestCredentialProbeAgentPathIsAdjacentToController(t *testing.T) {
	base := t.TempDir()
	path, err := credentialProbeAgentPath(filepath.Join(base, "krasis-control.exe"))
	if err != nil || path != filepath.Join(base, "krasis-agent.exe") {
		t.Fatalf("agent path = %q, err = %v", path, err)
	}
	path, err = credentialProbeAgentPath(filepath.Join(base, "krasis.exe"))
	if err != nil || path != filepath.Join(base, "krasis-agent.exe") {
		t.Fatalf("new UI agent path = %q, err = %v", path, err)
	}
	for _, invalid := range []string{"krasis-control.exe", filepath.Join(base, "other.exe")} {
		if _, err := credentialProbeAgentPath(invalid); err == nil {
			t.Fatalf("accepted invalid controller %q", invalid)
		}
	}
}
