package residentagent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/peerupdate"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/truststore"
)

func TestDiagnosticCollectionExposesStorageLimitWithoutQueueing(t *testing.T) {
	base := t.TempDir()
	dir, err := productidentity.PrepareDiagnostics(base)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 32; i++ {
		f, err := dir.CreateExclusive(fmt.Sprintf("support-request-%032x-local.json", i), 0600)
		if err != nil {
			t.Fatal(err)
		}
		if err = peercredentials.ProtectSecurityFileHandle(f); err != nil {
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	dir.Close()
	paths, _ := productidentity.ResolveWindowsPaths(base)
	m, err := peerupdate.Open(paths.Root, 202)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	peer := strings.Repeat("a", 64)
	trust := &stubTrustStore{document: truststore.Document{Peers: []truststore.PeerRecord{{Alias: "B", Fingerprint: peer}}}}
	o := &Operations{root: context.Background(), trust: trust, localAppData: base, updates: m}
	result, err := o.CollectDiagnostic(context.Background(), localapi.DiagnosticCollectionRequest{Action: "START", Peer: peer})
	if err != nil || result.Code != "storage_limit" || result.ID != "" || result.LocalPath != "" || result.PeerPath != "" {
		t.Fatal(result, err)
	}
}
