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
		return nil, ErrAuthority
	}
	identity := &processIdentity{handle: handle}
	accepted := false
	defer func() {
		if !accepted {
			_ = identity.close()
		}
	}()
	actual, err := windows.GetProcessId(handle)
	if err != nil || actual != pid || windows.ProcessIdToSessionId(pid, &identity.session) != nil {
		return nil, ErrAuthority
	}
	var image [32768]uint16
	size := uint32(len(image))
	if windows.QueryFullProcessImageName(handle, 0, &image[0], &size) != nil || size == 0 || size >= uint32(len(image)) {
		return nil, ErrAuthority
	}
	identity.image = windows.UTF16ToString(image[:size])
	var code uint32
	if windows.GetExitCodeProcess(handle, &code) != nil || code != 259 {
		return nil, ErrAuthority
	}
	accepted = true
	return identity, nil
}
