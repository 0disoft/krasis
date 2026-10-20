//go:build windows

package truststore

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"
)

// A second OS process must share the reader lock without preventing the next
// exclusive revocation from taking effect. In-process mutex tests cannot prove
// the Windows handle sharing and LockFileEx contract used by agent and UI.
func TestTrustReadAcrossProcessesPreservesRevocation(t *testing.T) {
	const helperDirectory = "KRASIS_TEST_TRUST_READ_DIRECTORY"
	if directory := os.Getenv(helperDirectory); directory != "" {
		store, err := New(directory)
		if err != nil {
			t.Fatal(err)
		}
		_, _, release, err := store.openReadLocked()
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		fmt.Fprintln(os.Stdout, "LOCKED")
		var stop [1]byte
		if _, err := io.ReadFull(os.Stdin, stop[:]); err != nil {
			t.Fatal(err)
		}
		return
	}

	now := time.Now().UTC()
	peer, certificate := replacementCertificate(t, now)
	store := mustStore(t)
	if _, err := store.ImportWithCertificate(peer, certificate.Certificate[0], "peer", false, now); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestTrustReadAcrossProcessesPreservesRevocation$")
	command.Env = append(os.Environ(), helperDirectory+"="+store.directory)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			cancel()
			_ = command.Wait()
		}
	}()
	ready := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(stdout).ReadString('\n')
		if err == nil && line != "LOCKED\n" {
			err = errors.New("reader helper did not acquire shared lock")
		}
		ready <- err
	}()
	select {
	case err := <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("reader helper did not become ready", ctx.Err())
	}
	if _, err := store.PeerCertificate(peer, now); err != nil {
		t.Fatal("another process's reader blocked certificate validation", err)
	}
	if _, err := store.Revoke(peer.CertificateSHA256, now); !errors.Is(err, ErrStoreBusy) {
		t.Fatal("revocation bypassed another process's live reader", err)
	}
	if _, err := stdin.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	err = command.Wait()
	waited = true
	if err != nil {
		t.Fatal("reader helper did not exit cleanly", err)
	}
	if _, err := store.Revoke(peer.CertificateSHA256, now); err != nil {
		t.Fatal("revocation remained blocked after reader exit", err)
	}
	if _, err := store.PeerCertificate(peer, now); !errors.Is(err, ErrPeerRevoked) {
		t.Fatal("validation accepted peer after revocation", err)
	}
}
