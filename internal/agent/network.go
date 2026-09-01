package agent

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/transport"
)

type ListenSource struct {
	listener         net.Listener
	tlsConfig        *tls.Config
	networkPolicy    ListenNetworkPolicy
	handshakeTimeout time.Duration
	preAuthSlots     chan struct{}
	preAuthAdmission *preAuthAdmission
	preAuthMetrics   preAuthMetricCounters
	shutdown         context.Context
	cancel           context.CancelFunc
	roundMu          sync.Mutex
	round            *listenRound
	acceptErr        error
	workers          sync.WaitGroup
	closeOnce        sync.Once
	closeErr         error
}

const (
	DefaultPreAuthHandshakeTimeout = 5 * time.Second
	MaxConcurrentPreAuthHandshakes = 4
)

type listenRound struct {
	ctx    context.Context
	cancel context.CancelFunc
	ready  chan listenResult
}

type listenResult struct {
	connection Connection
	err        error
}

func NewListenSource(address string, tlsConfig *tls.Config) (*ListenSource, error) {
	return NewListenSourceWithPolicy(
		address,
		tlsConfig,
		ListenNetworkTrustedPrivate,
		DefaultPreAuthAdmissionConfig(),
	)
}

func NewListenSourceWithPolicy(
	address string,
	tlsConfig *tls.Config,
	networkPolicy ListenNetworkPolicy,
	admissionConfig PreAuthAdmissionConfig,
) (*ListenSource, error) {
	if address == "" || tlsConfig == nil {
		return nil, errors.New("listen address and TLS config are required")
	}
	if err := networkPolicy.validate(); err != nil {
		return nil, err
	}
	if err := admissionConfig.validate(); err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	source, err := newListenSourceWithPolicy(
		listener,
		tlsConfig,
		DefaultPreAuthHandshakeTimeout,
		MaxConcurrentPreAuthHandshakes,
		networkPolicy,
		admissionConfig,
	)
	if err != nil {
		return nil, errors.Join(err, listener.Close())
	}
	return source, nil
}

func newListenSource(listener net.Listener, tlsConfig *tls.Config, handshakeTimeout time.Duration, maxPreAuth int) (*ListenSource, error) {
	return newListenSourceWithPolicy(
		listener,
		tlsConfig,
		handshakeTimeout,
		maxPreAuth,
		ListenNetworkTrustedPrivate,
		DefaultPreAuthAdmissionConfig(),
	)
}

func newListenSourceWithPolicy(
	listener net.Listener,
	tlsConfig *tls.Config,
	handshakeTimeout time.Duration,
	maxPreAuth int,
	networkPolicy ListenNetworkPolicy,
	admissionConfig PreAuthAdmissionConfig,
) (*ListenSource, error) {
	if listener == nil || tlsConfig == nil || handshakeTimeout <= 0 || maxPreAuth <= 0 {
		return nil, errors.New("listener, TLS config, positive handshake timeout, and pre-auth limit are required")
	}
	if err := networkPolicy.validate(); err != nil {
		return nil, err
	}
	admission, err := newPreAuthAdmission(admissionConfig)
	if err != nil {
		return nil, err
	}
	shutdown, cancel := context.WithCancel(context.Background())
	source := &ListenSource{
		listener:         listener,
		tlsConfig:        tlsConfig.Clone(),
		networkPolicy:    networkPolicy,
		handshakeTimeout: handshakeTimeout,
		preAuthSlots:     make(chan struct{}, maxPreAuth),
		preAuthAdmission: admission,
		shutdown:         shutdown,
		cancel:           cancel,
	}
	source.workers.Add(1)
	go source.acceptLoop()
	return source, nil
}

func (source *ListenSource) Address() string {
	if source == nil || source.listener == nil {
		return ""
	}
	return source.listener.Addr().String()
}

func (source *ListenSource) NetworkPolicy() ListenNetworkPolicy {
	if source == nil {
		return ""
	}
	return source.networkPolicy
}

func (source *ListenSource) PreAuthMetrics() PreAuthMetrics {
	if source == nil {
		return PreAuthMetrics{}
	}
	return source.preAuthMetrics.snapshot()
}

func (source *ListenSource) Next(ctx context.Context) (Connection, error) {
	if source == nil || source.listener == nil || source.tlsConfig == nil || source.shutdown == nil || ctx == nil {
		return nil, errors.New("listen source is not initialized")
	}
	roundCtx, cancel := context.WithCancel(source.shutdown)
	round := &listenRound{ctx: roundCtx, cancel: cancel, ready: make(chan listenResult)}
	source.roundMu.Lock()
	if source.acceptErr != nil {
		err := source.acceptErr
		source.roundMu.Unlock()
		cancel()
		return nil, err
	}
	if source.round != nil {
		source.roundMu.Unlock()
		cancel()
		return nil, errors.New("listen source already has a pending Next call")
	}
	source.round = round
	source.roundMu.Unlock()
	defer func() {
		source.roundMu.Lock()
		if source.round == round {
			source.round = nil
		}
		source.roundMu.Unlock()
		cancel()
	}()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-source.shutdown.Done():
		return nil, errors.New("listen source is closed")
	case result := <-round.ready:
		return result.connection, result.err
	}
}

func (source *ListenSource) Close() error {
	if source == nil || source.listener == nil {
		return nil
	}
	source.closeOnce.Do(func() {
		if source.cancel != nil {
			source.cancel()
		}
		source.closeErr = source.listener.Close()
	})
	source.workers.Wait()
	return source.closeErr
}

func (source *ListenSource) acceptLoop() {
	defer source.workers.Done()
	for {
		connection, err := source.listener.Accept()
		if err != nil {
			if source.shutdown.Err() == nil {
				source.roundMu.Lock()
				source.acceptErr = err
				round := source.round
				source.roundMu.Unlock()
				if round != nil {
					source.deliver(round, listenResult{err: err})
				}
			}
			return
		}
		source.roundMu.Lock()
		round := source.round
		source.roundMu.Unlock()
		if round == nil {
			_ = connection.Close()
			continue
		}
		decision := source.preAuthAdmission.allow(connection.RemoteAddr(), time.Now())
		if decision != preAuthAdmitted {
			source.preAuthMetrics.recordDecision(decision)
			_ = connection.Close()
			continue
		}
		select {
		case source.preAuthSlots <- struct{}{}:
			source.preAuthMetrics.recordDecision(preAuthAdmitted)
			source.workers.Add(1)
			go source.authenticate(round, connection)
		default:
			source.preAuthMetrics.rejectedBusy.Add(1)
			_ = connection.Close()
		}
	}
}

func (source *ListenSource) authenticate(round *listenRound, connection net.Conn) {
	defer source.workers.Done()
	defer func() { <-source.preAuthSlots }()
	handshakeCtx, cancel := context.WithTimeout(round.ctx, source.handshakeTimeout)
	defer cancel()
	peerConnection, err := transport.NewPeerConnection(handshakeCtx, tls.Server(connection, source.tlsConfig.Clone()))
	if err != nil {
		source.preAuthMetrics.handshakeFailed.Add(1)
		_ = connection.Close()
		return
	}
	source.preAuthMetrics.authenticated.Add(1)
	if !source.deliver(round, listenResult{connection: peerConnection}) {
		_ = peerConnection.Close()
	}
}

func (*ListenSource) deliver(round *listenRound, result listenResult) bool {
	select {
	case round.ready <- result:
		return true
	case <-round.ctx.Done():
		return false
	}
}

type DialSource struct {
	address     string
	tlsConfig   *tls.Config
	dialTimeout time.Duration
}

func NewDialSource(address string, tlsConfig *tls.Config, dialTimeout time.Duration) (*DialSource, error) {
	if address == "" || tlsConfig == nil || dialTimeout <= 0 {
		return nil, errors.New("dial address, TLS config, and positive timeout are required")
	}
	return &DialSource{address: address, tlsConfig: tlsConfig.Clone(), dialTimeout: dialTimeout}, nil
}

func (source *DialSource) Next(ctx context.Context) (Connection, error) {
	if source == nil || source.address == "" || source.tlsConfig == nil || source.dialTimeout <= 0 {
		return nil, errors.New("dial source is not initialized")
	}
	dialer := net.Dialer{Timeout: source.dialTimeout}
	connection, err := dialer.DialContext(ctx, "tcp", source.address)
	if err != nil {
		return nil, err
	}
	peerConnection, err := transport.NewPeerConnection(ctx, tls.Client(connection, source.tlsConfig.Clone()))
	if err != nil {
		return nil, closeConnectionOnError(connection, err)
	}
	return peerConnection, nil
}

func (*DialSource) Close() error { return nil }

func closeConnectionOnError(connection net.Conn, cause error) error {
	if connection == nil {
		return cause
	}
	return errors.Join(cause, connection.Close())
}
