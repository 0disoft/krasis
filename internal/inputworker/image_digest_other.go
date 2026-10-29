//go:build !windows

package inputworker

func newImageDigest() (*imageDigest, error) { return newGoImageDigest() }
