//go:build windows

package localapi

import (
	"context"
	"encoding/binary"
	"net"
	"net/http"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

var runtimeTCPTable = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetExtendedTcpTable")

type nativeRuntimeExit struct{ handle windows.Handle }

// Kernel listener ownership plus a fresh authenticated nonce bind this handle
// before STOP. Neither a disappeared API nor a released singleton proves exit.
// No PID/path enters the control schema, and this handle grants no kill rights.
func pinRuntimeExit(ctx context.Context, client *Client, instance string) (runtimeExit, error) {
	if ctx == nil || client == nil || !validLoopbackAddress(client.credentials.Endpoint.Address) || len(instance) != 32 {
		return nil, ErrAuthentication
	}
	_, portText, _ := net.SplitHostPort(client.credentials.Endpoint.Address)
	port, _ := strconv.ParseUint(portText, 10, 16)
	pid, err := lookupRuntimeListener(ctx, uint16(port))
	if err != nil {
		return nil, err
	}
	if pid == windows.GetCurrentProcessId() {
		return nil, ErrAuthentication
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return nil, ErrTransport
	}
	owner := &nativeRuntimeExit{handle: handle}
	accepted := false
	defer func() {
		if !accepted {
			_ = owner.Close()
		}
	}()
	// Do not authenticate through a pooled connection to a retired listener or
	// through proxies/redirects. This one connection goes to the fixed loopback
	// endpoint, under the same protected local API token and caller deadline.
	transport := &http.Transport{DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	probe := &Client{credentials: client.credentials, http: &http.Client{Transport: transport, Timeout: clientRequestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	info, err := probe.Runtime(ctx)
	if err != nil {
		return nil, err
	}
	if info.Instance != instance {
		return nil, ErrAuthentication
	}
	again, err := lookupRuntimeListener(ctx, uint16(port))
	if err != nil {
		return nil, err
	}
	state, err := windows.WaitForSingleObject(handle, 0)
	if err != nil || again != pid || state != uint32(windows.WAIT_TIMEOUT) {
		return nil, ErrAuthentication
	}
	accepted = true
	return owner, nil
}

func (owner *nativeRuntimeExit) Wait(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		state, err := windows.WaitForSingleObject(owner.handle, 20)
		if err != nil {
			return ErrTransport
		}
		if state == windows.WAIT_OBJECT_0 {
			return nil
		}
		if state != uint32(windows.WAIT_TIMEOUT) {
			return ErrTransport
		}
	}
}

func (owner *nativeRuntimeExit) Close() error {
	if owner.handle == 0 {
		return nil
	}
	err := windows.CloseHandle(owner.handle)
	owner.handle = 0
	if err != nil {
		return ErrTransport
	}
	return nil
}

func lookupRuntimeListener(ctx context.Context, port uint16) (uint32, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if runtimeTCPTable.Find() != nil {
		return 0, ErrTransport
	}
	var size uint32
	code, _, _ := runtimeTCPTable.Call(0, uintptr(unsafe.Pointer(&size)), 0, windows.AF_INET, 3, 0) // TCP_TABLE_OWNER_PID_LISTENER
	if code != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) {
		return 0, ErrTransport
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if size < 4 || size > 8<<20 {
			return 0, ErrTransport
		}
		buffer := make([]byte, size)
		code, _, _ = runtimeTCPTable.Call(uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)), 0, windows.AF_INET, 3, 0)
		if code == uintptr(windows.ERROR_INSUFFICIENT_BUFFER) {
			continue
		}
		if code != 0 || size > uint32(len(buffer)) {
			return 0, ErrTransport
		}
		return runtimeListenerPID(buffer[:size], port)
	}
	return 0, ErrTransport
}

// MIB_TCPROW_OWNER_PID consists of six DWORDs; the local address and port
// bytes use network order. Inspect only the exact IPv4 loopback listener.
func runtimeListenerPID(data []byte, port uint16) (uint32, error) {
	if len(data) < 4 || port == 0 {
		return 0, ErrTransport
	}
	count := binary.LittleEndian.Uint32(data[:4])
	if uint64(count) > uint64((len(data)-4)/24) {
		return 0, ErrTransport
	}
	var pid uint32
	for i := uint32(0); i < count; i++ {
		row := data[4+int(i)*24 : 4+int(i+1)*24]
		if binary.LittleEndian.Uint32(row[:4]) != 2 || row[4] != 127 || row[5] != 0 || row[6] != 0 || row[7] != 1 || binary.BigEndian.Uint16(row[8:10]) != port {
			continue
		}
		if pid != 0 || binary.LittleEndian.Uint32(row[20:24]) == 0 {
			return 0, ErrTransport
		}
		pid = binary.LittleEndian.Uint32(row[20:24])
	}
	if pid == 0 {
		return 0, ErrUnavailable
	}
	return pid, nil
}
