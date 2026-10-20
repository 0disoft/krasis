package netobserve

import (
	"context"
	"errors"
	"net"
	"syscall"
)

// A socket access denial is observable; its policy cause is not. Never infer
// firewall/WFP/profile changes from timeout, reset, or access denied alone.
func NetworkError(err error) string {
	if err == nil {
		return "ok"
	}
	if errors.Is(err, context.Canceled) {
		return "network_canceled"
	}
	var n net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &n) && n.Timeout() {
		return "network_timeout"
	}
	if code := platformNetworkError(err); code != "" {
		return code
	}
	switch {
	case errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM):
		return "network_access_denied"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "network_refused"
	case errors.Is(err, syscall.ENETUNREACH), errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETDOWN):
		return "network_unreachable"
	case errors.Is(err, syscall.ECONNRESET):
		return "network_reset"
	case errors.Is(err, syscall.ECONNABORTED):
		return "network_aborted"
	default:
		return "network_failed"
	}
}

func validNetworkError(s string) bool {
	switch s {
	case "ok", "network_canceled", "network_timeout", "network_access_denied", "network_refused", "network_unreachable", "network_reset", "network_aborted", "network_failed":
		return true
	}
	return false
}
