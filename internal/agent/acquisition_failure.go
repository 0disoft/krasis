package agent

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/netobserve"
	"github.com/0disoft/krasis/internal/transport"
)

func describeAcquisitionFailure(err error) FailureDescriptor {
	if errors.Is(err, transport.ErrPeerAuthentication) {
		return FailureDescriptor{Component: "peer-acquire", Phase: "authentication", Code: transport.AuthenticationFailureCode(err)}
	}
	code := netobserve.NetworkError(err)
	return FailureDescriptor{Component: "peer-acquire", Phase: "network", Code: code}
}

// The product supplies a bounded, nonblocking logger. Failed unauthenticated
// peers are rate limited by reason and never become an authenticated session.
func (source *ListenSource) SetAuthenticationFailureObserver(observer func(string)) {
	source.roundMu.Lock()
	defer source.roundMu.Unlock()
	source.authenticationObserver = observer
}
func (source *ListenSource) observeAuthenticationFailure(err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	code := transport.AuthenticationFailureCode(err)
	source.roundMu.Lock()
	observer := source.authenticationObserver
	if observer == nil {
		source.roundMu.Unlock()
		return
	}
	if source.authenticationLast == nil {
		source.authenticationLast = make(map[string]time.Time)
	}
	now := time.Now()
	if now.Sub(source.authenticationLast[code]) < time.Second {
		source.roundMu.Unlock()
		return
	}
	source.authenticationLast[code] = now
	source.roundMu.Unlock()
	observer(code)
}
