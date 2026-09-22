//go:build !windows

package wininput

func CurrentDisplayGeometry() DisplayGeometry { return DisplayGeometry{} }
