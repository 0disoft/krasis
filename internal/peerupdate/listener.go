package peerupdate

import (
	"crypto/tls"
	"errors"
	"net"
	"sync"
)

// Five independent HTTP lanes plus room for overlapping/idle connections.
const maxAuxiliaryConnections = 8

func listenAuxiliary(address string, config *tls.Config) (net.Listener, error) {
	// Preserve tls.Listen's fail-closed configuration check before binding.
	if config == nil || len(config.Certificates) == 0 && config.GetCertificate == nil && config.GetConfigForClient == nil {
		return nil, errors.New("TLS server certificate is required")
	}
	raw, err := net.Listen("tcp4", address)
	if err != nil {
		return nil, err
	}
	bounded := &auxiliaryListener{Listener: raw, slots: make(chan struct{}, maxAuxiliaryConnections)}
	// Limit raw sockets before net/http starts a TLS handshake goroutine. TLS
	// remains the outer listener, so Serve still receives a real *tls.Conn.
	return tls.NewListener(bounded, config), nil
}

type auxiliaryListener struct {
	net.Listener
	slots chan struct{}
}

func (l *auxiliaryListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		select {
		case l.slots <- struct{}{}:
			return &auxiliaryConnection{Conn: c, slots: l.slots}, nil
		default:
			_ = c.Close() // Reject overflow before allocating a TLS/HTTP owner.
		}
	}
}

type auxiliaryConnection struct {
	net.Conn
	slots chan struct{}
	once  sync.Once
	err   error
}

func (c *auxiliaryConnection) Close() error {
	c.once.Do(func() {
		c.err = c.Conn.Close()
		<-c.slots
	})
	return c.err
}
