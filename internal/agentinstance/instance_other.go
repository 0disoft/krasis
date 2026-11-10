//go:build !windows && !linux

package agentinstance

func acquire(string) (*Lease, error) { return nil, ErrUnsupported }
