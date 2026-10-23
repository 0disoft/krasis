package residentagent

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/truststore"
)

func TestManagementReplacesChangedConnectionOwnership(t *testing.T) {
	for _, change := range []string{"local-key", "alias", "address", "peer"} {
		t.Run(change, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				alias := "peer"
				prepared := productConnection{
					peer:     transport.AuthenticatedPeerContext{CertificateSHA256: "paired", Role: transport.AuthenticatedPeerConnector},
					profile:  truststore.ConnectionProfile{ConnectionSettings: truststore.ConnectionSettings{Address: "127.0.0.1:45872"}},
					localKey: "first-key",
				}
				valid := true
				var runs []context.Context
				ticks, done := make(chan time.Time), make(chan struct{})
				go func() {
					defer close(done)
					maintainUpdateConnections(ctx, func() (truststore.Document, error) {
						return truststore.Document{Peers: []truststore.PeerRecord{{Alias: alias, Fingerprint: prepared.peer.CertificateSHA256}}}, nil
					}, func(string) (productConnection, error) {
						if !valid {
							return productConnection{}, errors.New("current trust unavailable")
						}
						return prepared, nil
					}, func(run context.Context, gotAlias string, got productConnection) {
						if len(runs) > 0 && runs[len(runs)-1].Err() == nil {
							t.Error("replacement started before canceling obsolete authorization")
						}
						if gotAlias != alias || got.localKey != prepared.localKey {
							t.Error("replacement retained obsolete connection inputs")
						}
						runs = append(runs, run)
					}, ticks)
				}()
				synctest.Wait()
				poll := func() { ticks <- time.Now(); synctest.Wait() }
				poll()
				if len(runs) != 1 {
					t.Fatalf("unchanged connection restarted: %d", len(runs))
				}
				switch change {
				case "local-key":
					prepared.localKey = "renewed-key"
				case "alias":
					alias = "renamed-peer"
				case "address":
					prepared.profile.Address = "127.0.0.1:45875"
				case "peer":
					prepared.peer.CertificateSHA256 = "replacement-peer"
				}
				poll()
				if len(runs) != 2 || runs[0].Err() == nil {
					t.Errorf("changed connection not replaced: runs=%d old=%v", len(runs), runs[0].Err())
				}
				valid = false
				poll()
				if runs[len(runs)-1].Err() == nil {
					t.Error("failed trust refresh retained management authority")
				}
				cancel()
				<-done
			})
		})
	}
}
