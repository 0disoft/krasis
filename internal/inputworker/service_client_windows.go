//go:build windows

package inputworker

import "golang.org/x/sys/windows"

// Only process facts available with limited-query rights are read here.
// LocalSystem identity is established by protected SCM configuration and the
// pinned service image, which validates its own token before becoming ready.
func inspectServiceProcess(pid uint32) (*processIdentity, error) {
	if pid == 0 {
		return nil, ErrAuthority
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return nil, connectionFailed("service_process_open", err)
	}
	identity := &processIdentity{handle: handle}
	accepted := false
	defer func() {
		if !accepted {
			_ = identity.close()
		}
	}()
	actual, err := windows.GetProcessId(handle)
	if err != nil || actual != pid {
		return nil, connectionFailed("service_process_metadata", err)
	}
	if err := windows.ProcessIdToSessionId(pid, &identity.session); err != nil {
		return nil, connectionFailed("service_process_metadata", err)
	}
	var image [32768]uint16
	size := uint32(len(image))
	if err := windows.QueryFullProcessImageName(handle, 0, &image[0], &size); err != nil {
		return nil, connectionFailed("service_process_metadata", err)
	}
	if size == 0 || size >= uint32(len(image)) {
		return nil, connectionFailed("service_process_metadata", ErrAuthority)
	}
	identity.image = windows.UTF16ToString(image[:size])
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return nil, connectionFailed("service_process_metadata", err)
	}
	if code != 259 {
		return nil, connectionFailed("service_process_metadata", ErrAuthority)
	}
	accepted = true
	return identity, nil
}
