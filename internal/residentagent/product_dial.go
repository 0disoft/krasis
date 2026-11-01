package residentagent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/netobserve"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/truststore"
)

type boundedProductDial struct {
	observeDial   func(netobserve.DialAttempt)
	load          func() (truststore.ConnectionProfile, error)
	save          func(truststore.ConnectionProfile, string) error
	discover      func(context.Context) ([]string, error)
	observe       func(string)
	lastDiscovery time.Time
	fingerprint   string
	tlsConfig     *tls.Config
	anchor        *x509.Certificate
}

func (source *boundedProductDial) event(code string) {
	if source.observe != nil {
		source.observe(code)
	}
}

func (source *boundedProductDial) Next(ctx context.Context) (agent.Connection, error) {
	return source.next(ctx)
}

// Existing profile/discovery I/O is injected on source. These optional seams
// cover the clock, authenticated transport I/O and an additional abort signal.
// Next retains the ConnectionSource interface and uses production defaults.
type productDialOptions struct {
	now    func() time.Time
	dial   func(context.Context, string, time.Duration) (agent.Connection, error)
	signal context.Context
}

func (source *boundedProductDial) next(ctx context.Context, options ...productDialOptions) (agent.Connection, error) {
	if ctx == nil {
		return nil, errors.New("connection context is required")
	}
	opt := productDialOptions{now: time.Now, dial: source.dialAddress}
	if len(options) > 0 {
		if options[0].now != nil {
			opt.now = options[0].now
		}
		if options[0].dial != nil {
			opt.dial = options[0].dial
		}
		opt.signal = options[0].signal
	}
	bounded, cancel := context.WithDeadline(ctx, opt.now().Add(12*time.Second))
	defer cancel()
	if opt.signal != nil {
		stop := context.AfterFunc(opt.signal, cancel)
		defer stop()
	}
	check := func(run context.Context) error {
		if err := run.Err(); err != nil {
			return err
		}
		if opt.signal != nil && opt.signal.Err() != nil {
			return context.Canceled
		}
		// Timer delivery may lag behind native I/O completion or scheduling.
		if deadline, ok := run.Deadline(); ok && !opt.now().Before(deadline) {
			return context.DeadlineExceeded
		}
		return nil
	}
	if err := check(bounded); err != nil {
		return nil, err
	}
	profile, err := source.load()
	if stopped := check(bounded); stopped != nil {
		return nil, stopped
	}
	if err != nil {
		return nil, err
	}
	if profile.Fingerprint != source.fingerprint {
		return nil, errors.New("connection profile peer changed during reconnect")
	}
	dial := func(address string, timeout time.Duration) (agent.Connection, error) {
		if err := check(bounded); err != nil {
			return nil, err
		}
		attempt, stop := context.WithDeadline(bounded, opt.now().Add(timeout))
		defer stop()
		connection, err := opt.dial(attempt, address, timeout)
		if stopped := check(attempt); stopped != nil {
			err = stopped
		}
		if err != nil && connection != nil {
			err = errors.Join(err, connection.Close())
			connection = nil
		}
		if err == nil && connection == nil {
			err = errors.New("dial returned no connection")
		}
		return connection, err
	}
	connection, original := dial(profile.Address, 5*time.Second)
	if stopped := check(bounded); stopped != nil {
		if connection != nil {
			stopped = errors.Join(stopped, connection.Close())
		}
		return nil, errors.Join(stopped, original)
	}
	if original == nil || source.discover == nil || source.save == nil {
		return connection, original
	}
	if opt.now().Sub(source.lastDiscovery) < 15*time.Second {
		return nil, original
	}
	source.lastDiscovery = opt.now()
	source.event("address_discovery_started")
	if err := check(bounded); err != nil {
		return nil, err
	}
	candidates, err := source.discover(bounded)
	if stopped := check(bounded); stopped != nil {
		return nil, stopped
	}
	if err != nil {
		source.event("address_discovery_unavailable")
		return nil, original
	}
	_, port, _ := net.SplitHostPort(profile.Address)
	for i, address := range candidates {
		if err := check(bounded); err != nil {
			return nil, err
		}
		if i >= 8 {
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
			if stopped := check(bounded); stopped != nil {
				return nil, errors.Join(stopped, err)
			}
			source.event("address_candidate_rejected")
			continue
		}
		actual, err := connection.AuthenticatedPeer()
		if stopped := check(bounded); stopped != nil {
			return nil, errors.Join(stopped, connection.Close())
		}
		if err != nil || actual.CertificateSHA256 != source.fingerprint || actual.Role != transport.AuthenticatedPeerListener {
			_ = connection.Close()
			source.event("address_candidate_rejected")
			continue
		}
		// Recheck the alias binding after TLS. Saving also compares identity/address
		// under the trust-store lock, preserving concurrent layout edits.
		current, err := source.load()
		if stopped := check(bounded); stopped != nil {
			return nil, errors.Join(stopped, connection.Close())
		}
		if err != nil || current.Fingerprint != profile.Fingerprint || current.Address != profile.Address {
			_ = connection.Close()
			return nil, errors.New("connection profile changed during discovery")
		}
		err = source.save(profile, address)
		if stopped := check(bounded); stopped != nil {
			return nil, errors.Join(stopped, connection.Close())
		}
		if err != nil {
			_ = connection.Close()
			source.event("address_save_failed")
			return nil, err
		}
		source.event("address_discovery_confirmed")
		if stopped := check(bounded); stopped != nil {
			return nil, errors.Join(stopped, connection.Close())
		}
		return connection, nil
	}
	source.event("address_discovery_exhausted")
	if stopped := check(bounded); stopped != nil {
		return nil, stopped
	}
	return nil, original
}

func (source *boundedProductDial) dialAddress(ctx context.Context, address string, timeout time.Duration) (agent.Connection, error) {
	var anchors []*x509.Certificate
	if source.anchor != nil {
		anchors = []*x509.Certificate{source.anchor}
	}
	d, err := agent.NewDialSource(address, source.tlsConfig, timeout, anchors...)
	if err != nil {
		return nil, err
	}
	d.ObserveDial = source.observeDial
	return d.Next(ctx)
}

func (*boundedProductDial) Close() error { return nil }
