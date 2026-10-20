//go:build windows

package netobserve

import (
	"errors"
	"golang.org/x/sys/windows"
)

func platformNetworkError(err error) string {
	switch {
	case errors.Is(err, windows.WSAEACCES), errors.Is(err, windows.ERROR_ACCESS_DENIED):
		return "network_access_denied"
	case errors.Is(err, windows.WSAECONNREFUSED):
		return "network_refused"
	case errors.Is(err, windows.WSAENETDOWN), errors.Is(err, windows.WSAENETUNREACH), errors.Is(err, windows.WSAEHOSTUNREACH):
		return "network_unreachable"
	case errors.Is(err, windows.WSAECONNRESET), errors.Is(err, windows.WSAENETRESET):
		return "network_reset"
	case errors.Is(err, windows.WSAECONNABORTED):
		return "network_aborted"
	case errors.Is(err, windows.WSAETIMEDOUT):
		return "network_timeout"
	}
	return ""
}
