//go:build !windows

package agentinstance

func acquire(string) (*Lease, error) { return nil, ErrUnsupported }
