package agent

import (
	"context"
	"crypto/tls"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/transport"
)

func TestListenSourceRecordsCapacityAndHandshakeTimeout(t *testing.T) {
	serverTLS := networkTestServerTLS(t)
	policy := DefaultPreAuthPolicy()
	policy.HandshakeTimeout = 500 * time.Millisecond
	policy.MaxConcurrent = 1
	policy.PerAddressBurst = 8

	source, err := NewListenSourceWithPolicy("127.0.0.1:0", serverTLS, policy)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	nextDone := make(chan error, 1)
	go func() {
		connection, nextErr := source.Next(ctx)
		if connection != nil {
			_ = connection.Close()
		}
		nextDone <- nextErr
	}()
	waitForListenRound(t, source)

	stalled, err := net.DialTimeout("tcp", source.Address(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer stalled.Close()
	if _, err := stalled.Write([]byte{0x16}); err != nil {
		t.Fatal(err)
	}
	waitForPreAuthCondition(t, source, func(metrics PreAuthMetrics) bool {
		return len(source.preAuthSlots) == 1
	})

	excess, err := net.DialTimeout("tcp", source.Address(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = excess.Close()
	waitForPreAuthCondition(t, source, func(metrics PreAuthMetrics) bool {
		return metrics.RejectedCapacity == 1
	})
	waitForPreAuthCondition(t, source, func(metrics PreAuthMetrics) bool {
		return metrics.HandshakeTimeout == 1
	})

	cancel()
	select {
	case <-nextDone:
	case <-time.After(time.Second):
		t.Fatal("Next did not stop after cancellation")
	}
}

func TestListenSourceRateLimitsBeforeStartingAnotherHandshake(t *testing.T) {
	serverTLS := networkTestServerTLS(t)
	policy := DefaultPreAuthPolicy()
	policy.HandshakeTimeout = time.Second
	policy.PerAddressBurst = 1
	policy.PerAddressRefillInterval = time.Minute

	source, err := NewListenSourceWithPolicy("127.0.0.1:0", serverTLS, policy)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		connection, _ := source.Next(ctx)
		if connection != nil {
			_ = connection.Close()
		}
	}()
	waitForListenRound(t, source)

	first, err := net.DialTimeout("tcp", source.Address(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err := first.Write([]byte{0x16}); err != nil {
		t.Fatal(err)
	}
	waitForPreAuthCondition(t, source, func(metrics PreAuthMetrics) bool {
		return len(source.preAuthSlots) == 1
	})

	second, err := net.DialTimeout("tcp", source.Address(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = second.Close()
	waitForPreAuthCondition(t, source, func(metrics PreAuthMetrics) bool {
		return metrics.RejectedRateLimited == 1 && metrics.RejectedCapacity == 0
	})
}

func TestListenSourceCountsAuthenticatedPeerWithoutPersistingAddress(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "credentials")
	if _, err := peercredentials.Generate(directory, "127.0.0.1", time.Now()); err != nil {
		t.Fatal(err)
	}
	roots := loadAgentRoots(t, filepath.Join(directory, "listener", "ca.pem"))
	serverTLS, err := transport.NewServerTLSConfig(loadAgentIdentity(t, filepath.Join(directory, "listener")), roots)
	if err != nil {
		t.Fatal(err)
	}
	clientTLS, err := transport.NewClientTLSConfig(loadAgentIdentity(t, filepath.Join(directory, "connector")), roots, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	source, err := NewListenSource("127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	dialer, err := NewDialSource(source.Address(), clientTLS, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	accepted := make(chan Connection, 1)
	go func() {
		connection, _ := source.Next(ctx)
		accepted <- connection
	}()
	client, err := dialer.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server := <-accepted
	if server == nil {
		t.Fatal("listener did not return the authenticated peer")
	}
	defer server.Close()

	waitForPreAuthCondition(t, source, func(metrics PreAuthMetrics) bool {
		return metrics.Accepted == 1
	})
	if got := source.PreAuthMetrics(); got != (PreAuthMetrics{Accepted: 1}) {
		t.Fatalf("unexpected aggregate metrics: %+v", got)
	}
}

func networkTestServerTLS(t *testing.T) *tls.Config {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "credentials")
	if _, err := peercredentials.Generate(directory, "127.0.0.1", time.Now()); err != nil {
		t.Fatal(err)
	}
	roots := loadAgentRoots(t, filepath.Join(directory, "listener", "ca.pem"))
	serverTLS, err := transport.NewServerTLSConfig(loadAgentIdentity(t, filepath.Join(directory, "listener")), roots)
	if err != nil {
		t.Fatal(err)
	}
	return serverTLS
}

func waitForPreAuthCondition(t *testing.T, source *ListenSource, condition func(PreAuthMetrics) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		metrics := source.PreAuthMetrics()
		if condition(metrics) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("pre-auth condition not reached: %+v", source.PreAuthMetrics())
}
