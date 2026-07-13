package labcli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/truststore"
)

func TestTrustImportListAndRevoke(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "bundle")
	if _, err := peercredentials.Generate(bundle, "peer.test", time.Now()); err != nil {
		t.Fatal(err)
	}
	storeDirectory := filepath.Join(bundle, "listener", "trust")
	peerCertificate := filepath.Join(bundle, "listener", "peer.pem")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := runTrust([]string{
		"import", "--store", storeDirectory, "--peer-cert", peerCertificate,
		"--alias", "connector", "--allow-inbound-control",
	}, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("import code = %d, stderr = %q", code, stderr.String())
	}
	store, err := truststore.New(storeDirectory)
	if err != nil {
		t.Fatal(err)
	}
	document, err := store.List()
	if err != nil || len(document.Peers) != 1 {
		t.Fatalf("document = %+v, error = %v", document, err)
	}
	fingerprint := document.Peers[0].Fingerprint
	stdout.Reset()
	stderr.Reset()
	if code := runTrust([]string{"list", "--store", storeDirectory, "--json"}, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("list code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"schemaVersion":"krasis.trust-store/v1"`) || !strings.Contains(stdout.String(), fingerprint) {
		t.Fatalf("list output = %q", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := runTrust([]string{"revoke", "--store", storeDirectory, "--fingerprint", fingerprint}, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("revoke code = %d, stderr = %q", code, stderr.String())
	}
	document, err = store.List()
	if err != nil || document.Peers[0].RevokedAt == nil {
		t.Fatalf("revoked document = %+v, error = %v", document, err)
	}
}

func TestTrustImportRequiresExplicitInboundPermission(t *testing.T) {
	_, err := parseTrustImportArgs([]string{
		"--store", "trust", "--peer-cert", "peer.pem", "--alias", "peer",
	})
	if err == nil {
		t.Fatal("parseTrustImportArgs() accepted missing permission acknowledgement")
	}
}

func TestTrustListRejectsDuplicateJSONFlag(t *testing.T) {
	if _, err := parseTrustListArgs([]string{"--store", "trust", "--json", "--json"}); err == nil {
		t.Fatal("parseTrustListArgs() accepted duplicate --json")
	}
}
