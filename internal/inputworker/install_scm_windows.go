//go:build windows

package inputworker

import (
	"context"
	"encoding/binary"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type installServiceConfig struct {
	typeID, start, errorControl  uint32
	binary, account, group, deps string
	tag                          uint32
}

type installSCM struct {
	manager windows.Handle
	root    string
}

// SCM writes pointer-bearing native structures into this storage. A []byte
// backing array may be only byte-aligned on a Go stack; Windows RPC can fault
// inside QueryServiceConfig before returning an error for such an address.
// uint64 storage guarantees native alignment on the supported windows/amd64
// target, including when the compiler keeps this bounded buffer on the stack.
func installServiceBuffer() []byte {
	words := make([]uint64, 8192/8)
	return unsafe.Slice((*byte)(unsafe.Pointer(&words[0])), len(words)*8)
}

func (manager *installSCM) command() string {
	return windows.EscapeArg(filepath.Join(manager.root, "Input", "krasis-input.exe")) + " --service"
}

func validInstallServiceConfig(config installServiceConfig, command string) bool {
	return config.typeID == windows.SERVICE_WIN32_OWN_PROCESS && config.start == windows.SERVICE_DEMAND_START &&
		config.errorControl == windows.SERVICE_ERROR_NORMAL && config.binary == command &&
		strings.EqualFold(config.account, "LocalSystem") && config.group == "" && config.deps == "" && config.tag == 0
}

// Parse kernel-filled pointers through offsets in the owned buffer. Never
// convert an arbitrary returned uintptr into a Go pointer under checkptr.
func installServiceString(buffer []byte, pointer *uint16) (string, error) {
	if pointer == nil {
		return "", nil
	}
	if len(buffer) == 0 {
		return "", ErrAuthority
	}
	base := uintptr(unsafe.Pointer(&buffer[0]))
	address := uintptr(unsafe.Pointer(pointer))
	if address < base || address-base >= uintptr(len(buffer)) || (address-base)%2 != 0 {
		return "", ErrAuthority
	}
	offset := int(address - base)
	var characters []uint16
	for offset+2 <= len(buffer) {
		character := binary.LittleEndian.Uint16(buffer[offset : offset+2])
		if character == 0 {
			return windows.UTF16ToString(characters), nil
		}
		characters = append(characters, character)
		offset += 2
	}
	return "", ErrAuthority
}

func readInstallServiceConfig(handle windows.Handle) (installServiceConfig, error) {
	buffer := installServiceBuffer()
	config := (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&buffer[0]))
	var needed uint32
	if err := windows.QueryServiceConfig(handle, config, uint32(len(buffer)), &needed); err != nil {
		return installServiceConfig{}, installNativeError("service_config", ErrAuthority, err)
	}
	if needed > uint32(len(buffer)) {
		return installServiceConfig{}, ErrAuthority
	}
	result := installServiceConfig{typeID: config.ServiceType, start: config.StartType, errorControl: config.ErrorControl, tag: config.TagId}
	for _, field := range []struct {
		pointer *uint16
		output  *string
	}{
		{config.BinaryPathName, &result.binary}, {config.ServiceStartName, &result.account},
		{config.LoadOrderGroup, &result.group}, {config.Dependencies, &result.deps},
	} {
		value, err := installServiceString(buffer, field.pointer)
		if err != nil {
			return installServiceConfig{}, err
		}
		*field.output = value
	}
	return result, nil
}

func protectedInstallService(descriptor *windows.SECURITY_DESCRIPTOR) error {
	if descriptor == nil {
		return ErrAuthority
	}
	owner, _, err := descriptor.Owner()
	if err != nil || !administratorSID(owner) {
		return ErrAuthority
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil || dacl.AceCount > 256 {
		return ErrAuthority
	}
	const reads = windows.READ_CONTROL | windows.SERVICE_QUERY_CONFIG | windows.SERVICE_QUERY_STATUS | windows.SERVICE_INTERROGATE
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(dacl, index, &ace) != nil || ace == nil || ace.Header.AceSize < 16 {
			return ErrAuthority
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return ErrAuthority
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !sid.IsValid() || sid.Len() > int(ace.Header.AceSize)-8 || uint32(ace.Mask)&^uint32(reads) != 0 && !administratorSID(sid) {
			return ErrAuthority
		}
	}
	return nil
}

func validateInstallService(handle windows.Handle, command string) error {
	config, err := readInstallServiceConfig(handle)
	if err != nil {
		return err
	}
	if !validInstallServiceConfig(config, command) {
		return ErrAuthority
	}
	descriptor, err := windows.GetSecurityInfo(handle, windows.SE_SERVICE, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return installNativeError("service_security", ErrAuthority, err)
	}
	if err := protectedInstallService(descriptor); err != nil {
		return err
	}
	// No automatic crash restart, custom command or trigger may race the
	// installer after it has obtained a clean stop receipt.
	buffer := installServiceBuffer()
	var needed uint32
	if err := windows.QueryServiceConfig2(handle, windows.SERVICE_CONFIG_FAILURE_ACTIONS, &buffer[0], uint32(len(buffer)), &needed); err != nil {
		return installNativeError("service_config", ErrAuthority, err)
	}
	if needed > uint32(len(buffer)) {
		return ErrAuthority
	}
	actions := (*windows.SERVICE_FAILURE_ACTIONS)(unsafe.Pointer(&buffer[0]))
	commandText, err := installServiceString(buffer, actions.Command)
	if err != nil || commandText != "" || actions.ActionsCount != 0 {
		return ErrAuthority
	}
	if err := windows.QueryServiceConfig2(handle, windows.SERVICE_CONFIG_TRIGGER_INFO, &buffer[0], uint32(len(buffer)), &needed); err != nil {
		return installNativeError("service_config", ErrAuthority, err)
	}
	if needed > uint32(len(buffer)) || binary.LittleEndian.Uint32(buffer[:4]) != 0 {
		return ErrAuthority
	}
	return nil
}

func (manager *installSCM) open(ctx context.Context, access uint32) (windows.Handle, error) {
	if manager == nil || manager.manager == 0 || ctx == nil || ctx.Err() != nil {
		return 0, ErrAuthority
	}
	name, err := windows.UTF16PtrFromString(ServiceName)
	if err != nil {
		return 0, ErrAuthority
	}
	handle, err := windows.OpenService(manager.manager, name, access|windows.SERVICE_QUERY_CONFIG|windows.SERVICE_QUERY_STATUS|windows.READ_CONTROL)
	if err != nil {
		if err == windows.ERROR_SERVICE_DOES_NOT_EXIST || err == windows.ERROR_SERVICE_MARKED_FOR_DELETE {
			return 0, err
		}
		return 0, installNativeError("service_open", ErrAuthority, err)
	}
	if err := validateInstallService(handle, manager.command()); err != nil {
		_ = windows.CloseServiceHandle(handle)
		return 0, err
	}
	return handle, nil
}

func installServiceStatus(handle windows.Handle) (windows.SERVICE_STATUS_PROCESS, error) {
	var status windows.SERVICE_STATUS_PROCESS
	var needed uint32
	if err := windows.QueryServiceStatusEx(handle, windows.SC_STATUS_PROCESS_INFO, (*byte)(unsafe.Pointer(&status)), uint32(unsafe.Sizeof(status)), &needed); err != nil {
		return status, installNativeError("service_status", ErrAuthority, err)
	}
	if status.ServiceType != windows.SERVICE_WIN32_OWN_PROCESS {
		return status, ErrAuthority
	}
	return status, nil
}

func (manager *installSCM) query(ctx context.Context) (windows.SERVICE_STATUS_PROCESS, bool, error) {
	handle, err := manager.open(ctx, 0)
	if err == windows.ERROR_SERVICE_DOES_NOT_EXIST {
		return windows.SERVICE_STATUS_PROCESS{}, false, nil
	}
	if err != nil {
		return windows.SERVICE_STATUS_PROCESS{}, false, err
	}
	defer windows.CloseServiceHandle(handle)
	status, err := installServiceStatus(handle)
	return status, true, err
}

func waitInstallService(ctx context.Context) error {
	timer := time.NewTimer(50 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ErrAuthority
	case <-timer.C:
		return nil
	}
}

func (manager *installSCM) process(status windows.SERVICE_STATUS_PROCESS, digest [32]byte) (*processIdentity, error) {
	identity, err := inspectProcess(status.ProcessId)
	if err != nil {
		return nil, ErrAuthority
	}
	accepted := false
	defer func() {
		if !accepted {
			_ = identity.close()
		}
	}()
	if identity.session != 0 || !identity.system || identity.integrity != 0x4000 || identity.uiAccess || identity.restricted || identity.appContainer || !strings.EqualFold(identity.image, filepath.Join(manager.root, "Input", "krasis-input.exe")) {
		return nil, ErrAuthority
	}
	image, err := lockImage(identity.image, digest)
	if err != nil {
		return nil, ErrAuthority
	}
	if protectedHandle(windows.Handle(image.Fd())) != nil || image.Close() != nil {
		_ = image.Close()
		return nil, ErrAuthority
	}
	accepted = true
	return identity, nil
}

func (manager *installSCM) ready(status windows.SERVICE_STATUS_PROCESS, digest [32]byte) error {
	if status.CurrentState != windows.SERVICE_RUNNING {
		return ErrAuthority
	}
	identity, err := manager.process(status, digest)
	if err != nil {
		return err
	}
	defer identity.close()
	record, err := readServiceRecord(manager.root)
	if err != nil || record.Phase != serviceReady || record.PID != status.ProcessId || record.Digest != digest {
		return ErrAuthority
	}
	state, err := windows.WaitForSingleObject(identity.handle, 0)
	if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		return ErrAuthority
	}
	return nil
}

func (manager *installSCM) start(ctx context.Context, digest [32]byte) error {
	if ctx == nil {
		return ErrAuthority
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	handle, err := manager.open(ctx, windows.SERVICE_START)
	if err != nil {
		return err
	}
	defer windows.CloseServiceHandle(handle)
	status, err := installServiceStatus(handle)
	if err != nil {
		return err
	}
	if status.CurrentState == windows.SERVICE_STOPPED {
		if err := windows.StartService(handle, 0, nil); err != nil {
			return installNativeError("service_start", ErrAuthority, err)
		}
	} else if status.CurrentState != windows.SERVICE_RUNNING && status.CurrentState != windows.SERVICE_START_PENDING {
		return ErrAuthority
	}
	for ctx.Err() == nil {
		status, err := installServiceStatus(handle)
		if err != nil {
			return err
		}
		if status.CurrentState == windows.SERVICE_STOPPED {
			if status.Win32ExitCode != 0 {
				return installNativeError("service_start", ErrAuthority, windows.Errno(status.Win32ExitCode))
			}
			return ErrAuthority
		}
		if status.CurrentState == windows.SERVICE_RUNNING && manager.ready(status, digest) == nil {
			return nil
		}
		if waitInstallService(ctx) != nil {
			break
		}
	}
	return ErrAuthority
}

func (manager *installSCM) cleanStopped(status windows.SERVICE_STATUS_PROCESS, digest [32]byte, pid uint32) error {
	if status.CurrentState != windows.SERVICE_STOPPED {
		return ErrRecovery
	}
	if status.Win32ExitCode == uint32(windows.ERROR_SERVICE_NEVER_STARTED) {
		return nil
	}
	record, err := readServiceRecord(manager.root)
	if err != nil || record.Phase != serviceClean || record.Digest != digest || pid != 0 && record.PID != pid {
		return ErrRecovery
	}
	return nil
}

func (manager *installSCM) stop(ctx context.Context, digest [32]byte) error {
	if ctx == nil {
		return ErrAuthority
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	handle, err := manager.open(ctx, windows.SERVICE_STOP)
	if err == windows.ERROR_SERVICE_DOES_NOT_EXIST {
		return nil
	}
	if err != nil {
		return err
	}
	defer windows.CloseServiceHandle(handle)
	var process *processIdentity
	defer func() {
		if process != nil {
			_ = process.close()
		}
	}()
	for ctx.Err() == nil {
		status, err := installServiceStatus(handle)
		if err != nil {
			return err
		}
		if status.CurrentState == windows.SERVICE_STOPPED {
			pid := uint32(0)
			if process != nil {
				pid = statusPID(process)
				if pid == 0 {
					return ErrRecovery
				}
			}
			if manager.cleanStopped(status, digest, pid) != nil {
				return ErrRecovery
			}
			if process != nil {
				state, err := windows.WaitForSingleObject(process.handle, 0)
				if err != nil {
					return ErrRecovery
				}
				if state == uint32(windows.WAIT_TIMEOUT) {
					if waitInstallService(ctx) != nil {
						return ErrRecovery
					}
					continue
				}
				if state != windows.WAIT_OBJECT_0 {
					return ErrRecovery
				}
			}
			return nil
		}
		if status.CurrentState == windows.SERVICE_RUNNING && process == nil {
			process, err = manager.process(status, digest)
			if err != nil {
				return err
			}
			var result windows.SERVICE_STATUS
			if err := windows.ControlService(handle, windows.SERVICE_CONTROL_STOP, &result); err != nil {
				return installNativeError("service_stop", ErrRecovery, err)
			}
		} else if status.CurrentState != windows.SERVICE_RUNNING && status.CurrentState != windows.SERVICE_START_PENDING && status.CurrentState != windows.SERVICE_STOP_PENDING {
			return ErrRecovery
		}
		if waitInstallService(ctx) != nil {
			break
		}
	}
	return ErrRecovery
}

func statusPID(process *processIdentity) uint32 {
	pid, err := windows.GetProcessId(process.handle)
	if err != nil {
		return 0
	}
	return pid
}

func (manager *installSCM) configure(ctx context.Context, present bool) error {
	if ctx == nil {
		return ErrAuthority
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for ctx.Err() == nil {
		access := uint32(0)
		if !present {
			access = windows.DELETE
		}
		handle, err := manager.open(ctx, access)
		if err == windows.ERROR_SERVICE_MARKED_FOR_DELETE {
			if waitInstallService(ctx) != nil {
				return ErrAuthority
			}
			continue
		}
		if err == windows.ERROR_SERVICE_DOES_NOT_EXIST {
			if !present {
				return nil
			}
			return manager.create(ctx)
		}
		if err != nil {
			return err
		}
		if present {
			return windows.CloseServiceHandle(handle)
		}
		status, err := installServiceStatus(handle)
		if err != nil || status.CurrentState != windows.SERVICE_STOPPED {
			_ = windows.CloseServiceHandle(handle)
			if err != nil {
				return err
			}
			return ErrAuthority
		}
		if err := windows.DeleteService(handle); err != nil {
			windows.CloseServiceHandle(handle)
			return installNativeError("service_delete", ErrAuthority, err)
		}
		_ = windows.CloseServiceHandle(handle)
		// Wait until SCM actually removes it before considering removal complete.
	}
	return ErrAuthority
}

func (manager *installSCM) create(ctx context.Context) error {
	if ctx.Err() != nil {
		return ErrAuthority
	}
	name, _ := windows.UTF16PtrFromString(ServiceName)
	display, _ := windows.UTF16PtrFromString("Krasis Input")
	command, err := windows.UTF16PtrFromString(manager.command())
	if err != nil {
		return ErrAuthority
	}
	access := uint32(windows.READ_CONTROL | windows.WRITE_DAC | windows.WRITE_OWNER | windows.DELETE | windows.SERVICE_QUERY_CONFIG | windows.SERVICE_QUERY_STATUS)
	handle, err := windows.CreateService(manager.manager, name, display, access, windows.SERVICE_WIN32_OWN_PROCESS, windows.SERVICE_DEMAND_START, windows.SERVICE_ERROR_NORMAL, command, nil, nil, nil, nil, nil)
	if err != nil {
		return installNativeError("service_create", ErrAuthority, err)
	}
	defer windows.CloseServiceHandle(handle)
	descriptor, err := windows.SecurityDescriptorFromString("O:BAG:BAD:P(A;;0xF01FF;;;SY)(A;;0xF01FF;;;BA)(A;;0x20005;;;BU)")
	if err != nil {
		_ = windows.DeleteService(handle)
		return ErrAuthority
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		_ = windows.DeleteService(handle)
		return ErrAuthority
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		windows.DeleteService(handle)
		return ErrAuthority
	}
	if err := windows.SetSecurityInfo(handle, windows.SE_SERVICE, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, owner, nil, dacl, nil); err != nil {
		_ = windows.DeleteService(handle)
		return installNativeError("service_security", ErrAuthority, err)
	}
	if err := validateInstallService(handle, manager.command()); err != nil {
		windows.DeleteService(handle)
		return err
	}
	return nil
}
