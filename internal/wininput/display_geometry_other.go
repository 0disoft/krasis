//go:build !windows && !linux

package wininput

func CurrentDisplayGeometry() DisplayGeometry { return DisplayGeometry{} }
