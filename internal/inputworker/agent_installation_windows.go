//go:build windows

package inputworker

import (
	"crypto/sha256"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// AgentLaunch pins the administrator-owned images until process creation.
// It selects only the fixed installed agent, never a path from peer metadata.
type AgentLaunch struct {
	installation *Installation
	path         string
}

func OpenAgentLaunch(expected [sha256.Size]byte) (*AgentLaunch, error) {
	if expected == ([sha256.Size]byte{}) {
		return nil, ErrAuthority
	}
	installation, err := LoadInstallation()
	if err != nil {
		return nil, err
	}
	agent, _, err := installation.digests()
	if err != nil || agent != expected {
		_ = installation.Close()
		return nil, ErrAuthority
	}
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		_ = installation.Close()
		return nil, ErrAuthority
	}
	return &AgentLaunch{installation: installation, path: filepath.Join(root, "Krasis", "Input", "krasis-agent.exe")}, nil
}

func (launch *AgentLaunch) Path() string {
	if launch == nil || launch.installation == nil {
		return ""
	}
	return launch.path
}
func (launch *AgentLaunch) Close() error {
	if launch == nil {
		return nil
	}
	return launch.installation.Close()
}

// CurrentAgentServiceDigest enables product input only from the installed,
// matching ordinary console agent. Portable agents retain conservative guards.
func CurrentAgentServiceDigest() ([sha256.Size]byte, error) {
	installation, err := LoadInstallation()
	if err != nil {
		return [sha256.Size]byte{}, ErrAuthority
	}
	defer installation.Close()
	agent, service, err := installation.digests()
	if err != nil {
		return [sha256.Size]byte{}, ErrAuthority
	}
	self, err := inspectProcess(windows.GetCurrentProcessId())
	if err != nil {
		return [sha256.Size]byte{}, ErrAuthority
	}
	defer self.close()
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil || !validAgentIdentity(self, windows.WTSGetActiveConsoleSessionId()) || !strings.EqualFold(self.image, filepath.Join(root, "Krasis", "Input", "krasis-agent.exe")) {
		return [sha256.Size]byte{}, ErrAuthority
	}
	image, err := lockImage(self.image, agent)
	if err != nil {
		return [sha256.Size]byte{}, ErrAuthority
	}
	if image.Close() != nil {
		return [sha256.Size]byte{}, ErrAuthority
	}
	return service, nil
}
