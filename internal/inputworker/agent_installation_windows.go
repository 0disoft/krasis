//go:build windows

package inputworker

import (
	"context"
	"crypto/sha256"
	"path/filepath"
	"strings"
	"time"

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
	agent, service, err := installation.digests()
	if err != nil || agent != expected {
		_ = installation.Close()
		return nil, ErrAuthority
	}
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		_ = installation.Close()
		return nil, ErrAuthority
	}
	if !installedServiceLaunchReady(filepath.Join(root, "Krasis"), service) {
		_ = installation.Close()
		return nil, ErrAuthority
	}
	return &AgentLaunch{installation: installation, path: filepath.Join(root, "Krasis", "Input", "krasis-agent.exe")}, nil
}

// This read-only launch gate never starts/repairs a service or grants an input
// lease. A stopped or partially installed service leaves the UI's ordinary
// portable-agent path usable. The installed agent still authenticates each
// privileged lease independently after launch.
func installedServiceLaunchReady(root string, digest [sha256.Size]byte) bool {
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false
	}
	defer windows.CloseServiceHandle(manager)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	scm := &installSCM{manager: manager, root: root}
	status, present, err := scm.query(ctx)
	if err != nil || !present {
		return false
	}
	record, err := readServiceRecord(root)
	return err == nil && readyInstalledServiceStatus(status, record, digest)
}

func readyInstalledServiceStatus(status windows.SERVICE_STATUS_PROCESS, record serviceRecord, digest [sha256.Size]byte) bool {
	return status.ServiceType == windows.SERVICE_WIN32_OWN_PROCESS && status.CurrentState == windows.SERVICE_RUNNING && status.ProcessId != 0 &&
		digest != ([sha256.Size]byte{}) && record.Phase == serviceReady && record.PID == status.ProcessId && record.Digest == digest && record.Run != (Binding{})
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
	if !installedServiceLaunchReady(filepath.Join(root, "Krasis"), service) {
		return [sha256.Size]byte{}, ErrAuthority
	}
	return service, nil
}
