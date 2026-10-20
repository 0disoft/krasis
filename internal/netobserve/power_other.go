//go:build !windows

package netobserve

func powerStatus() (Power, Saver) { return PowerUnknown, SaverUnknown }
