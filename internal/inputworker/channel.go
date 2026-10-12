package inputworker

import (
	"net"
	"sync"
)

// Channel ties one authenticated kernel pipe to its process proof. Its private
// fields prevent callers from pairing a proof with an unrelated input stream.
type Channel struct {
	stream    net.Conn
	authority *Authority
	closeOnce sync.Once
	closeErr  error
}

func (channel *Channel) Close() error {
	if channel == nil {
		return nil
	}
	channel.closeOnce.Do(func() {
		var streamErr error
		if channel.stream != nil {
			streamErr = channel.stream.Close()
		}
		proofErr := channel.authority.Close()
		if streamErr != nil || proofErr != nil {
			channel.closeErr = ErrAuthority
		}
	})
	return channel.closeErr
}
