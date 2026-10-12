//go:build windows

package inputworker

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// AgentLaunch pins the administrator-owned images until process creation.
// It selects only the fixed installed agent, never a path from peer metadata.
type AgentLaunch struct {
	installation *Installation
	path         string
}

// Read-only observation of the installer's exclusive file lock. It never
// creates/deletes a lock or journal. This also fences app restart during setup.
func InstallationRefreshRunning() (bool, error) {
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return false, ErrAuthority
	}
	return installationRefreshRunning(filepath.Join(root, "Krasis", "INPUT-TRANSACTION.lock"))
}

func installationRefreshRunning(path string) (bool, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false, ErrAuthority
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, ErrAuthority
	}
	defer windows.CloseHandle(handle)
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(handle, &info) != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 || info.NumberOfLinks != 1 {
		return false, ErrAuthority
	}
	return false, nil
}

// Read-only identity comparison. This does not grant a lease or require service
// readiness. Close every image pin before executing the elevated installer.
func InstallationMatchesAgent(expected [sha256.Size]byte) (bool, error) {
	installation, err := LoadInstallation()
	if err != nil {
		return false, err
	}
	agent, _, err := installation.digests()
	closeErr := installation.Close()
	if err != nil {
		return false, err
	}
	if closeErr != nil {
		return false, closeErr
	}
	return expected != ([sha256.Size]byte{}) && agent == expected, nil
}

func OpenAgentLaunch(expected [sha256.Size]byte) (*AgentLaunch, error) {
	return openAgentLaunch(expected, 0)
}

// Only startup waits for the already-starting service. Health reads stay cheap;
// neither path starts a service or changes its installed configuration.
func OpenAgentLaunchForStartup(expected [sha256.Size]byte) (*AgentLaunch, error) {
	return openAgentLaunch(expected, 5*time.Second)
}

func openAgentLaunch(expected [sha256.Size]byte, wait time.Duration) (*AgentLaunch, error) {
	if expected == ([sha256.Size]byte{}) {
		return nil, ErrAuthority
	}
	if running, err := InstallationRefreshRunning(); err != nil || running {
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
	if err := installedServiceLaunchErrorWithin(filepath.Join(root, "Krasis"), service, wait); err != nil {
		_ = installation.Close()
		return nil, err
	}
	return &AgentLaunch{installation: installation, path: filepath.Join(root, "Krasis", "Input", "krasis-agent.exe")}, nil
}

// This read-only launch gate never starts/repairs a service or grants an input
// lease. A stopped or partially installed service leaves the UI's ordinary
// portable-agent path usable. The installed agent still authenticates each
// privileged lease independently after launch.
func installedServiceLaunchReady(root string, digest [sha256.Size]byte) bool {
	return installedServiceLaunchReadyWithin(root, digest, 0)
}

func installedServiceLaunchReadyWithin(root string, digest [sha256.Size]byte, wait time.Duration) bool {
	return installedServiceLaunchErrorWithin(root, digest, wait) == nil
}

func installedServiceLaunchErrorWithin(root string, digest [sha256.Size]byte, wait time.Duration) error {
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return &InstallationHealthError{Stage: "service_manager", Cause: err}
	}
	defer windows.CloseServiceHandle(manager)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second+wait)
	defer cancel()
	scm := &installSCM{manager: manager, root: root}
	deadline := time.Now().Add(wait)
	for {
		status, present, err := scm.query(ctx)
		if err != nil {
			return &InstallationHealthError{Stage: "service_query", Cause: err}
		}
		if !present {
			return &InstallationHealthError{Stage: "service_missing", Cause: ErrAuthority}
		}
		if status.CurrentState != windows.SERVICE_RUNNING && status.CurrentState != windows.SERVICE_START_PENDING {
			cause := error(ErrAuthority)
			if status.Win32ExitCode != 0 {
				cause = errors.Join(cause, syscall.Errno(status.Win32ExitCode))
			}
			return &InstallationHealthError{Stage: "service_state", Stopped: status.CurrentState == windows.SERVICE_STOPPED, Cause: cause}
		}
		record, err := readServiceRecord(root)
		if err == nil && readyInstalledServiceStatus(status, record, digest) {
			return nil
		}
		if wait == 0 || !time.Now().Before(deadline) || waitInstallService(ctx) != nil {
			if err != nil {
				return &InstallationHealthError{Stage: "service_record", Cause: err}
			}
			return &InstallationHealthError{Stage: "service_identity", Cause: ErrAuthority}
		}
	}
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
	service, err := OpenAgentService()
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	digest := service.digest
	if err := service.Close(); err != nil {
		return [sha256.Size]byte{}, err
	}
	return digest, nil
}

// AgentService owns verified installation pins for one product connection.
// Per-lease authentication still checks the kernel pipe owner, SCM binding,
// service configuration and image ACL. Close before installer/update work.
type AgentService struct {
	installation *Installation
	digest       [sha256.Size]byte
}

func (service *AgentService) Close() error {
	if service == nil {
		return nil
	}
	return service.installation.Close()
}

func (service *AgentService) ConnectLease(ctx context.Context, lease Lease) (*Client, error) {
	if service == nil || service.digest == ([sha256.Size]byte{}) {
		return nil, ErrAuthority
	}
	_, digest, err := service.installation.digests()
	if err != nil || digest != service.digest {
		return nil, ErrAuthority
	}
	return connectInstalledLease(ctx, digest, lease, service.installation)
}

func OpenAgentService() (*AgentService, error) {
	installation, err := LoadInstallation()
	if err != nil {
		return nil, ErrAuthority
	}
	accepted := false
	defer func() {
		if !accepted {
			_ = installation.Close()
		}
	}()
	agent, service, err := installation.digests()
	if err != nil {
		return nil, ErrAuthority
	}
	self, err := inspectProcess(windows.GetCurrentProcessId())
	if err != nil {
		return nil, ErrAuthority
	}
	defer self.close()
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil || !validAgentIdentity(self, windows.WTSGetActiveConsoleSessionId()) || !strings.EqualFold(self.image, filepath.Join(root, "Krasis", "Input", "krasis-agent.exe")) {
		return nil, ErrAuthority
	}
	image, err := pinIdentityImage(installation, self.image, agent)
	if err != nil {
		return nil, ErrAuthority
	}
	if image.Close() != nil {
		return nil, ErrAuthority
	}
	if !installedServiceLaunchReady(filepath.Join(root, "Krasis"), service) {
		return nil, ErrAuthority
	}
	accepted = true
	return &AgentService{installation: installation, digest: service}, nil
}
