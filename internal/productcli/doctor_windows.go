//go:build windows

package productcli

import (
	"context"
	"debug/buildinfo"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/0disoft/krasis/internal/inputworker"
	"github.com/0disoft/krasis/internal/launchrecord"
	"golang.org/x/sys/windows"
)

func inspectDoctorHost(ctx context.Context, appData, agentBuild string) doctorHost {
	host := doctorHost{UI: doctorComponent{State: "UNKNOWN"}, LaunchRecord: doctorComponent{State: "UNKNOWN"}, InstalledAgent: doctorComponent{State: "UNKNOWN"}, Service: doctorComponent{State: "UNKNOWN"}, AgentLocation: "UNKNOWN"}
	if ctx.Err() != nil {
		return host
	}
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return host
	}
	installed := filepath.Join(root, "Krasis", "Input")
	host.Service = inspectDoctorService(filepath.Join(installed, "krasis-input.exe"))
	if _, err := os.Lstat(installed); errors.Is(err, os.ErrNotExist) {
		host.InstalledAgent.State = "NOT_INSTALLED"
	} else if err == nil {
		installation, err := inputworker.LoadInstallation()
		if err != nil {
			host.InstalledAgent.State = "INVALID"
		} else {
			host.InstalledAgent.Build = doctorImageBuild(filepath.Join(installed, "krasis-agent.exe"))
			host.InstalledAgent.State = "VERIFIED"
			if host.InstalledAgent.Build == "" {
				host.InstalledAgent.State = "UNKNOWN"
			}
			if installation.Close() != nil {
				host.InstalledAgent.State = "UNKNOWN"
			}
		}
	}
	if ctx.Err() != nil {
		return host
	}
	images, unknown := doctorProcessImages(ctx)
	record, recordErr := launchrecord.Inspect(appData)
	switch {
	case recordErr == nil:
		host.LaunchRecord = doctorComponent{State: "VERIFIED", Generation: record.Generation, Build: record.Revision, PathStatus: "MATCHED"}
		if !validHex(record.Revision, 40) {
			host.LaunchRecord.State, host.LaunchRecord.Build = "UNKNOWN", ""
		}
	case errors.Is(recordErr, launchrecord.ErrMissing), errors.Is(recordErr, os.ErrNotExist):
		host.LaunchRecord.State = "NOT_FOUND"
	case errors.Is(recordErr, launchrecord.ErrInvalid):
		host.LaunchRecord.State = "INVALID"
	}
	uis := images["krasis.exe"]
	host.UI = doctorUIComponent(uis, unknown, record, recordErr, doctorImageBuild)
	if agentBuild == "" {
		host.AgentLocation = "UNAVAILABLE"
		return host
	}
	locations := map[string]bool{}
	for _, path := range images["krasis-agent.exe"] {
		if ctx.Err() != nil {
			return host
		}
		if doctorImageBuild(path) != agentBuild {
			continue
		}
		locations[strings.ToLower(filepath.Clean(path))] = true
	}
	if unknown {
		return host
	}
	if len(locations) > 1 {
		host.AgentLocation = "AMBIGUOUS"
	} else if len(locations) == 1 {
		host.AgentLocation = "PORTABLE"
		if locations[strings.ToLower(filepath.Join(installed, "krasis-agent.exe"))] {
			host.AgentLocation = "PROTECTED"
		}
	}
	return host
}

func doctorImageBuild(path string) string {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 64<<20 {
		return ""
	}
	build, err := buildinfo.ReadFile(path)
	if err != nil || !strings.HasPrefix(build.Path, "github.com/0disoft/krasis/cmd/") {
		return ""
	}
	for _, setting := range build.Settings {
		if setting.Key == "vcs.revision" && validHex(setting.Value, 40) {
			return setting.Value
		}
	}
	return ""
}

// Same-session image metadata only. Never read command lines, tokens or input.
// Helper children at the same path are de-duplicated; conflicting paths stay unknown.
func doctorProcessImages(ctx context.Context) (map[string][]string, bool) {
	images := map[string][]string{}
	var session uint32
	if windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &session) != nil {
		return images, true
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return images, true
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	err = windows.Process32First(snapshot, &entry)
	unknown := false
	for err == nil {
		if ctx.Err() != nil {
			return images, true
		}
		name := strings.ToLower(windows.UTF16ToString(entry.ExeFile[:]))
		if name == "krasis.exe" || name == "krasis-agent.exe" {
			var other uint32
			if windows.ProcessIdToSessionId(entry.ProcessID, &other) != nil {
				unknown = true
			} else if other == session {
				path, readErr := doctorProcessPath(entry.ProcessID)
				if readErr != nil {
					unknown = true
				} else {
					found := false
					for _, old := range images[name] {
						if strings.EqualFold(path, old) {
							found = true
						}
					}
					if !found {
						images[name] = append(images[name], path)
					}
				}
			}
		}
		err = windows.Process32Next(snapshot, &entry)
	}
	return images, unknown || !errors.Is(err, windows.ERROR_NO_MORE_FILES)
}

func doctorProcessPath(pid uint32) (string, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, 32768)
	length := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &length); err != nil {
		return "", err
	}
	if length == 0 || length > uint32(len(buffer)) {
		return "", os.ErrInvalid
	}
	return windows.UTF16ToString(buffer[:length]), nil
}

func inspectDoctorService(expectedImage string) doctorComponent {
	result := doctorComponent{State: "UNKNOWN", PathStatus: "UNKNOWN"}
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return result
	}
	defer windows.CloseServiceHandle(manager)
	name, _ := windows.UTF16PtrFromString("KrasisInput")
	service, err := windows.OpenService(manager, name, windows.SERVICE_QUERY_STATUS|windows.SERVICE_QUERY_CONFIG)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return doctorComponent{State: "NOT_INSTALLED"}
	}
	if err != nil {
		return result
	}
	defer windows.CloseServiceHandle(service)
	var status windows.SERVICE_STATUS_PROCESS
	var needed uint32
	if windows.QueryServiceStatusEx(service, windows.SC_STATUS_PROCESS_INFO, (*byte)(unsafe.Pointer(&status)), uint32(unsafe.Sizeof(status)), &needed) != nil {
		return result
	}
	words := make([]uint64, 1024) // Native pointer-bearing structure must be aligned.
	buffer := unsafe.Slice((*byte)(unsafe.Pointer(&words[0])), len(words)*8)
	config := (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&buffer[0]))
	if windows.QueryServiceConfig(service, config, uint32(len(buffer)), &needed) != nil || needed > uint32(len(buffer)) {
		return result
	}
	command, ok := doctorServiceString(buffer, config.BinaryPathName)
	if !ok {
		return result
	}
	account, ok := doctorServiceString(buffer, config.ServiceStartName)
	if !ok {
		return result
	}
	result.PathStatus = "MISMATCH"
	if command == windows.EscapeArg(expectedImage)+" --service" && config.ServiceType == windows.SERVICE_WIN32_OWN_PROCESS && strings.EqualFold(account, "LocalSystem") {
		result.PathStatus = "MATCHED"
	}
	switch status.CurrentState {
	case windows.SERVICE_RUNNING:
		result.State = "RUNNING"
	case windows.SERVICE_STOPPED:
		result.State = "STOPPED"
	default:
		result.State = "PENDING"
	}
	return result
}

func doctorServiceString(buffer []byte, ptr *uint16) (string, bool) {
	if ptr == nil || len(buffer) == 0 {
		return "", false
	}
	base, address := uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(ptr))
	if address < base || address-base >= uintptr(len(buffer)) || (address-base)%2 != 0 {
		return "", false
	}
	var chars []uint16
	for offset := int(address - base); offset+2 <= len(buffer); offset += 2 {
		c := binary.LittleEndian.Uint16(buffer[offset : offset+2])
		if c == 0 {
			return windows.UTF16ToString(chars), true
		}
		chars = append(chars, c)
	}
	return "", false
}
