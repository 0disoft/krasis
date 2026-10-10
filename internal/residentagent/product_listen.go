package residentagent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/peerdiscovery"
	"github.com/0disoft/krasis/internal/truststore"
)

// Track the originally selected NIC. Never widen a saved bind to wildcard or
// silently move it to another adapter when the original interface disappears.
func interfaceForAddress(address string) int {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return 0
	}
	ip := net.ParseIP(host)
	if ip.To4() == nil || !ip.IsPrivate() {
		return 0
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return 0
	}
	for _, adapter := range interfaces {
		addresses, err := adapter.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addresses {
			if n, ok := a.(*net.IPNet); ok && n.IP.Equal(ip) {
				return adapter.Index
			}
		}
	}
	return 0
}

func chooseInterfaceAddress(address string, addresses []net.Addr) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", err
	}
	ip := net.ParseIP(host)
	if ip.To4() == nil || !ip.IsPrivate() {
		return address, nil
	}
	var candidates []string
	for _, a := range addresses {
		n, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if n.IP.Equal(ip) {
			return address, nil
		}
		if n.IP.To4() != nil && n.IP.IsPrivate() {
			candidates = append(candidates, n.IP.String())
		}
	}
	if len(candidates) != 1 {
		return "", errors.New("selected interface has no unique private address")
	}
	return net.JoinHostPort(candidates[0], port), nil
}

func resolveInterfaceAddress(address string, index int) (string, error) {
	if index == 0 {
		return address, nil
	}
	adapter, err := net.InterfaceByIndex(index)
	if err != nil || adapter.Flags&net.FlagUp == 0 {
		return "", errors.New("selected interface unavailable")
	}
	addresses, err := adapter.Addrs()
	if err != nil {
		return "", err
	}
	return chooseInterfaceAddress(address, addresses)
}

type productListen struct {
	mu                   sync.Mutex
	listener             *agent.ListenSource
	advertisement        io.Closer
	address, fingerprint string
	index                int
	tls                  *tls.Config
	anchors              []*x509.Certificate
	load                 func() (truststore.ConnectionProfile, error)
	save                 func(truststore.ConnectionProfile, string) error
	resolve              func(string, int) (string, error)
	observe              func(string)
	ctx                  context.Context
	cancel               context.CancelFunc
	done                 chan struct{}
	closed               bool
	invalidated          bool
}

func newProductListen(ctx context.Context, p truststore.ConnectionProfile, config *tls.Config, load func() (truststore.ConnectionProfile, error), save func(truststore.ConnectionProfile, string) error, observe func(string), anchors ...*x509.Certificate) (*productListen, error) {
	listener, err := agent.NewListenSource(p.Address, config, anchors...)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	s := &productListen{listener: listener, address: p.Address, fingerprint: p.Fingerprint, index: interfaceForAddress(p.Address), tls: config, load: load, save: save, resolve: resolveInterfaceAddress, observe: observe, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	s.anchors = anchors
	s.installObserver(listener)
	s.advertisement, err = peerdiscovery.Advertise(ctx, p.Address)
	if err != nil {
		s.event("address_advertisement_unavailable")
	}
	go s.watch()
	return s, nil
}

func (s *productListen) event(code string) {
	if s.observe != nil {
		s.observe(code)
	}
}
func (s *productListen) installObserver(listener *agent.ListenSource) {
	listener.SetAuthenticationFailureObserver(func(code string) { s.event("preauth_" + code) })
}

func (s *productListen) watch() {
	defer close(s.done)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			address, index, listener := s.address, s.index, s.listener
			s.mu.Unlock()
			updated, err := s.resolve(address, index)
			if err != nil || updated != address {
				s.mu.Lock()
				current := !s.closed && listener == s.listener
				s.mu.Unlock()
				if current {
					s.mu.Lock()
					if listener == s.listener {
						s.invalidated = true
					}
					s.mu.Unlock()
					_ = listener.Close()
				} // Wake a pending accept; recovery happens only in Next.
			}
		}
	}
}

func (s *productListen) Next(ctx context.Context) (agent.Connection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.closed || s.ctx.Err() != nil {
		s.mu.Unlock()
		return nil, context.Canceled
	}
	p, err := s.load()
	if err != nil || p.Fingerprint != s.fingerprint {
		s.mu.Unlock()
		return nil, errors.New("listener profile unavailable or peer changed")
	}
	index := s.index
	if p.Address != s.address {
		index = interfaceForAddress(p.Address)
	}
	address, err := s.resolve(p.Address, index)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	if address != s.address || s.invalidated {
		if s.invalidated {
			_ = s.listener.Close()
		}
		listener, err := agent.NewListenSource(address, s.tls, s.anchors...)
		if err != nil {
			s.mu.Unlock()
			return nil, err
		}
		s.installObserver(listener)
		if address != p.Address {
			if err = s.save(p, address); err != nil {
				_ = listener.Close()
				s.mu.Unlock()
				s.event("address_save_failed")
				return nil, err
			}
		}
		old, advertisement := s.listener, s.advertisement
		s.listener, s.address, s.index = listener, address, index
		s.invalidated = false
		if advertisement != nil {
			_ = advertisement.Close()
		}
		s.advertisement, err = peerdiscovery.Advertise(s.ctx, address)
		if err != nil {
			s.event("address_advertisement_unavailable")
		}
		_ = old.Close()
		s.event("address_listener_rebound")
	}
	listener := s.listener
	s.mu.Unlock()
	return listener.Next(ctx)
}

func (s *productListen) Close() error {
	s.cancel()
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	err := s.listener.Close()
	if s.advertisement != nil {
		err = errors.Join(err, s.advertisement.Close())
	}
	return err
}
