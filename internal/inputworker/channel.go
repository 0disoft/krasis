package inputworker

import (
	"net"
	"sync"
	"time"
)

// Channel ties one authenticated kernel pipe to its process proof. Its private
// fields prevent callers from pairing a proof with an unrelated input stream.
type Channel struct {
	stream           net.Conn
	authority        *Authority
	closeOnce        sync.Once
	closeErr         error
	queryMu          sync.Mutex
	query            *authorityQuery
	presenceQuery    *authorityQuery
	maintenanceQuery *authorityQuery // Optional heartbeat/advisory proof; never input.
	closing          bool
}

func (channel *Channel) Close() error {
	if channel == nil {
		return nil
	}
	channel.queryMu.Lock()
	channel.closing = true
	if channel.pendingQueriesLocked() {
		channel.authority.fenced.Store(true)
		// Interrupt wire work, but retain the pipe and proof handles used by
		// the read-only native query. Its owner closes them after it returns.
		if channel.stream != nil {
			_ = channel.stream.SetDeadline(time.Now())
		}
		channel.queryMu.Unlock()
		return ErrAuthority // Deferred retirement is not confirmed cleanup.
	}
	channel.queryMu.Unlock()
	return channel.closeResources()
}

func (channel *Channel) pendingQueriesLocked() bool {
	return channel.query != nil || channel.presenceQuery != nil || channel.maintenanceQuery != nil
}

func (channel *Channel) closeResources() error {
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
