package inputworker

import "net"

// Channel ties one authenticated kernel pipe to its process proof. Its private
// fields prevent callers from pairing a proof with an unrelated input stream.
type Channel struct {
	stream    net.Conn
	authority *Authority
}

func (channel *Channel) Close() error {
	if channel == nil {
		return nil
	}
	var streamErr error
	if channel.stream != nil {
		streamErr = channel.stream.Close()
	}
	proofErr := channel.authority.Close()
	if streamErr != nil || proofErr != nil {
		return ErrAuthority
	}
	return nil
}
