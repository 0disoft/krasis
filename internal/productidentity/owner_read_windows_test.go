//go:build windows

package productidentity

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/transport"
	"golang.org/x/sys/windows"
)

func TestOwnerTransportNeverCreatesMissingOrExpiredCredentials(t *testing.T) {
	base := t.TempDir()
	now := time.Now().UTC()
	paths, identity, _, err := BootstrapWindows(base, now)
	if err != nil {
		t.Fatal(err)
	}
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
	role := transport.AuthenticatedPeerListener
	if _, _, err := ReadOwnerTransport(base, owner, role, now); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("missing credential accepted", err)
	}
	file := filepath.Join(paths.Identity, "transport-LISTENER.json")
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read created credential")
	}
	store, _ := New(paths.Identity)
	cert, err := store.OpenTransport(role, now)
	if err != nil {
		t.Fatal(err)
	}
	public, readCert, err := ReadOwnerTransport(base, owner, role, now)
	if err != nil || public != identity.Public() || !bytes.Equal(cert.Certificate[0], readCert.Certificate[0]) {
		t.Fatal("existing credential changed", err)
	}
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadOwnerTransport(base, owner, role, cert.Leaf.NotAfter); !errors.Is(err, ErrTransportRenewalRequired) {
		t.Fatal("expired credential accepted", err)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("read renewed expired credential")
	}
}
