//go:build !windows

package netobserve

func platformNetworkError(error) string { return "" }
