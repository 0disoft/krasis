package residentagent

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/truststore"
)

type boundedProductDial struct {
	load          func() (truststore.ConnectionProfile, error)
	save          func(truststore.ConnectionProfile, string) error
	discover      func(context.Context) ([]string, error)
	observe       func(string)
	lastDiscovery time.Time
	fingerprint   string
	tlsConfig     *tls.Config
}

func (source *boundedProductDial) event(code string) {
	if source.observe != nil {
		source.observe(code)
	}
}

func (source *boundedProductDial) Next(ctx context.Context) (agent.Connection, error) {
	bounded, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	profile, err := source.load()
	if err != nil {
		return nil, err
	}
	if profile.Fingerprint != source.fingerprint {
		return nil, errors.New("connection profile peer changed during reconnect")
	}
	dial := func(address string, timeout time.Duration) (agent.Connection, error) {
		attempt, stop := context.WithTimeout(bounded, timeout)
		defer stop()
		d, err := agent.NewDialSource(address, source.tlsConfig, timeout)
		if err != nil {
			return nil, err
		}
		return d.Next(attempt)
	}
	connection, original := dial(profile.Address, 5*time.Second)
	if original == nil || bounded.Err() != nil || source.discover == nil || source.save == nil {
		return connection, original
	}
	if time.Since(source.lastDiscovery) < 15*time.Second {
		return nil, original
	}
	source.lastDiscovery = time.Now()
	source.event("address_discovery_started")
	candidates, err := source.discover(bounded)
	if err != nil {
		source.event("address_discovery_unavailable")
		return nil, original
	}
	_, port, _ := net.SplitHostPort(profile.Address)
	for i, address := range candidates {
		if i >= 8 || bounded.Err() != nil {
			break
		}
		settings := profile.ConnectionSettings
		settings.Address = address
		_, candidatePort, _ := net.SplitHostPort(address)
		if address == profile.Address || candidatePort != port || settings.Validate() != nil {
			continue
		}
		connection, err = dial(address, time.Second)
		if err != nil {
			source.event("address_candidate_rejected")
			continue
		}
		actual, err := connection.AuthenticatedPeer()
		if err != nil || actual.CertificateSHA256 != source.fingerprint || actual.Role != transport.AuthenticatedPeerListener {
			_ = connection.Close()
			source.event("address_candidate_rejected")
			continue
		}
		// Recheck the alias binding after TLS. Saving also compares identity/address
		// under the trust-store lock, preserving concurrent layout edits.
		current, err := source.load()
		if err != nil || current.Fingerprint != profile.Fingerprint || current.Address != profile.Address {
			_ = connection.Close()
			return nil, errors.New("connection profile changed during discovery")
		}
		if err = source.save(profile, address); err != nil {
			_ = connection.Close()
			source.event("address_save_failed")
			return nil, err
		}
		source.event("address_discovery_confirmed")
		return connection, nil
	}
	source.event("address_discovery_exhausted")
	return nil, original
}

func (*boundedProductDial) Close() error { return nil }
