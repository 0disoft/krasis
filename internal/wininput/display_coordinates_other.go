//go:build !windows && !linux

package wininput

func DisplayCoordinates() (uint32, uint32, int32) { return 0, 0, -1 }
