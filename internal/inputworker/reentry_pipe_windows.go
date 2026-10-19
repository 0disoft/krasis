//go:build windows

package inputworker

import "errors"

// Only the service's clean lease path calls this, after worker recovery has
// completed. Reserving a listener transfers no proof or input authority. Its
// next serial accept must still authenticate the actual client process.
func rebindServiceInputPipe(previous *Channel, name string) (*pipeConn, error) {
	if previous == nil {
		return nil, ErrAuthority
	}
	if err := previous.Close(); err != nil {
		return nil, errors.Join(ErrRecovery, err)
	}
	listener, err := makePipe(name)
	if err != nil {
		// Never proceed into optional work with uncertain endpoint ownership.
		return nil, errors.Join(ErrRecovery, err)
	}
	return listener, nil
}
