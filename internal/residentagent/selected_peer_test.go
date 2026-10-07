package residentagent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/truststore"
)

func TestSelectedIdentityIsCheckedAtAdmissionAndRetainedBySession(t *testing.T) {
	fingerprint := strings.Repeat("a", 64)
	store := &stubTrustStore{document: truststore.Document{Peers: []truststore.PeerRecord{{Alias: "peer", Fingerprint: fingerprint}}}}
	observed := make(chan string, 1)
	o := testOperations(t, func(ctx context.Context, _ string, _ agent.SupervisorObserver) error {
		observed <- localapi.PeerFingerprint(ctx)
		<-ctx.Done()
		return context.Cause(ctx)
	})
	o.trust = store
	result, err := o.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer", Fingerprint: strings.Repeat("b", 64)})
	if err != nil || result.Code != "selected_peer_changed" || o.sessionDone != nil {
		t.Fatal("admitted wrong selected identity", result, err)
	}
	o.preflight = func(string) error { store.document.Peers[0].Fingerprint = strings.Repeat("b", 64); return nil }
	result, err = o.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer", Fingerprint: fingerprint})
	if err != nil || result.Status != "COMPLETED" {
		t.Fatal(result, err)
	}
	if got := <-observed; got != fingerprint {
		t.Fatal("session rebound to changed alias", got)
	}
	if err := o.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestProductRunnerRejectsAliasRebindingBeforePreparingInput(t *testing.T) {
	paths, _, _, err := productidentity.BootstrapWindows(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	store, err := truststore.New(paths.Trust)
	if err != nil {
		t.Fatal(err)
	}
	current := strings.Repeat("b", 64)
	if _, err := store.Import(transport.AuthenticatedPeerContext{CertificateSHA256: current, Role: transport.AuthenticatedPeerListener}, "peer", true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetConnection("peer", truststore.ConnectionSettings{Address: "127.0.0.1:12345", Edge: "RIGHT"}); err != nil {
		t.Fatal(err)
	}
	ctx := localapi.WithPeerFingerprint(context.Background(), strings.Repeat("a", 64))
	// No identities, certificates or input adapter are supplied: rebinding must
	// fail before the product runner can use any of them.
	if err := productRunner(nil, store, nil)(ctx, "peer", nil); err == nil || err.Error() != "selected peer changed" {
		t.Fatal("runner did not reject changed selected identity", err)
	}
}
