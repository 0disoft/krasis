//go:build windows

package truststore

import (
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/productidentity"
	"golang.org/x/sys/windows"
)

func TestOwnerTrustSnapshotObservesRevocationAndAliasMismatch(t *testing.T) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	owner := user.User.Sid.String()
	base := t.TempDir()
	now := time.Now().UTC()
	paths, _, _, err := productidentity.BootstrapWindows(base, now)
	if err != nil {
		t.Fatal(err)
	}
	store, _ := New(paths.Trust)
	peer, cert := replacementCertificate(t, now)
	if _, err := store.ImportWithCertificate(peer, cert.Certificate[0], "B", true, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetConnection("B", ConnectionSettings{Address: "127.0.0.1:24800", Edge: "RIGHT"}); err != nil {
		t.Fatal(err)
	}
	got, profile, leaf, err := ReadOwnerConnection(base, owner, "B", peer.CertificateSHA256, now)
	if err != nil || got.Fingerprint != peer.CertificateSHA256 || profile.Fingerprint != peer.CertificateSHA256 || leaf == nil {
		t.Fatal("valid selection not read", err)
	}
	if _, _, _, err := ReadOwnerConnection(base, owner, "A", peer.CertificateSHA256, now); !errors.Is(err, ErrPeerNotFound) {
		t.Fatal("alias mismatch accepted", err)
	}
	_, _, release, err := store.openLocked(false)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, readErr := ReadOwnerConnection(base, owner, "B", peer.CertificateSHA256, now)
	release()
	if !errors.Is(readErr, ErrStoreBusy) {
		t.Fatal("read bypassed trust write lock", readErr)
	}
	if _, err := store.Revoke(peer.CertificateSHA256, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ReadOwnerConnection(base, owner, "B", peer.CertificateSHA256, now.Add(time.Second)); err == nil {
		t.Fatal("read restored revoked peer")
	}
}
