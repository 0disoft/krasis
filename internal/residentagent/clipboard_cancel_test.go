package residentagent

import (
	"context"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/peerupdate"
	"github.com/0disoft/krasis/internal/truststore"
)

type clipboardCancellingTrust struct {
	stubTrustStore
	cancel context.CancelFunc
}

func (s *clipboardCancellingTrust) List() (truststore.Document, error) {
	s.cancel() // Caller departs after the operation's first authorization check.
	return s.stubTrustStore.List()
}

func TestClipboardOperationPreservesCancellationAfterAuthorization(t *testing.T) {
	base := t.TempDir()
	if _, err := peercredentials.ProtectSecurityDirectory(base); err != nil {
		t.Fatal(err)
	}
	m, err := peerupdate.Open(base, 72)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	peer := strings.Repeat("b", 64)
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	text := "current copy"
	if _, err := m.SyncClipboard(peer, &text, ""); err != nil {
		t.Fatal(err)
	}
	_, id, size := m.ClipboardInfo(peer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &clipboardCancellingTrust{stubTrustStore: stubTrustStore{document: truststore.Document{
		Peers: []truststore.PeerRecord{{Fingerprint: peer, Alias: "peer"}},
	}}, cancel: cancel}
	o := testOperationsWithTrust(t, store)
	o.updates = m
	obsolete := "cancelled new copy"
	result, err := o.Clipboard(ctx, localapi.ClipboardRequest{Action: "SYNC", Peer: peer, Text: &obsolete, LocalChanged: true})
	if err != nil || result.Code != "rejected" || ctx.Err() == nil {
		t.Fatal("cancelled operation was acknowledged", result.Code, err)
	}
	if _, after, afterSize := m.ClipboardInfo(peer); after != id || afterSize != size {
		t.Fatal("operation dropped request cancellation before publishing")
	}
}
