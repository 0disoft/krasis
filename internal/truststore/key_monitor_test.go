package truststore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestKeyAnchorMonitorRereadsTrustAndCertificate(t *testing.T) {
	now := time.Now().UTC()
	peer, certificate := replacementCertificate(t, now)
	for _, failure := range []string{"none", "revoked", "missing", "corrupt", "zero_time"} {
		t.Run(failure, func(t *testing.T) {
			store := mustStore(t)
			if _, err := store.ImportWithCertificate(peer, certificate.Certificate[0], "peer", true, now); err != nil {
				t.Fatal(err)
			}
			check, err := store.NewKeyAnchorMonitor(peer, now)
			if err != nil {
				t.Fatal(err)
			}
			if err := check(now, NewReadTrace()); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(store.directory, peerCertificateFilename(peer.CertificateSHA256))
			switch failure {
			case "revoked":
				_, err = store.Revoke(peer.CertificateSHA256, now.Add(time.Second))
			case "missing":
				err = os.Remove(path)
			case "corrupt":
				err = os.WriteFile(path, []byte("invalid certificate"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			at := now.Add(time.Second)
			if failure == "zero_time" {
				at = time.Time{}
			}
			trace := NewReadTrace()
			err = check(at, trace)
			if failure == "none" {
				if err != nil {
					t.Fatal(err)
				}
				if _, ok := trace.Snapshot().Durations["certificate_match"]; !ok {
					t.Fatal("unchanged bytes not compared")
				}
			} else if err == nil {
				t.Fatal("cached crypto concealed trust or file change")
			}
			if failure == "revoked" && !errors.Is(err, ErrPeerRevoked) {
				t.Fatal(err)
			}
		})
	}
}

func TestPeerPolicyMonitorReadsCurrentPermissionWithoutMetadataCache(t *testing.T) {
	now := time.Now().UTC()
	peer, certificate := replacementCertificate(t, now)
	store := mustStore(t)
	if _, err := store.ImportWithCertificate(peer, certificate.Certificate[0], "peer", true, now); err != nil {
		t.Fatal(err)
	}
	monitor, err := store.NewPeerPolicyMonitor(peer, now)
	if err != nil {
		t.Fatal(err)
	}
	if allowed, err := monitor.InboundControlAllowed(peer, now); err != nil || !allowed {
		t.Fatal(allowed, err)
	}
	path := filepath.Join(store.directory, storeFilename)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Another Store models a separate caller. Keep length and mtime unchanged:
	// the extra boolean byte is offset by the shorter alias.
	other, err := New(store.directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Update(peer.CertificateSHA256, "pee", false, now); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("test must preserve metadata", err)
	}
	if allowed, err := monitor.InboundControlAllowed(peer, now); err != nil || allowed {
		t.Fatal("cached permission survived withdrawal", allowed, err)
	}
	// Withdrawing input permission does not invalidate the connection key.
	if err := monitor.CheckAnchor(now, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := other.Update(peer.CertificateSHA256, "peer", true, now); err != nil {
		t.Fatal(err)
	}
	if allowed, err := monitor.InboundControlAllowed(peer, now); err != nil || !allowed {
		t.Fatal("new permission not observed", allowed, err)
	}
	wrong := peer
	wrong.Role = "invalid"
	if allowed, err := monitor.InboundControlAllowed(wrong, now); err == nil || allowed {
		t.Fatal("monitor accepted a different peer", allowed, err)
	}
	if _, err := other.Revoke(peer.CertificateSHA256, now); err != nil {
		t.Fatal(err)
	}
	if allowed, err := monitor.InboundControlAllowed(peer, now); !errors.Is(err, ErrPeerRevoked) || allowed {
		t.Fatal("revoked peer accepted", allowed, err)
	}
}

func TestPeerPolicyMonitorRejectsMissingAndChangedCertificate(t *testing.T) {
	now := time.Now().UTC()
	peer, certificate := replacementCertificate(t, now)
	for _, missing := range []bool{false, true} {
		store := mustStore(t)
		if _, err := store.ImportWithCertificate(peer, certificate.Certificate[0], "peer", true, now); err != nil {
			t.Fatal(err)
		}
		monitor, err := store.NewPeerPolicyMonitor(peer, now)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(store.directory, peerCertificateFilename(peer.CertificateSHA256))
		if missing {
			err = os.Remove(path)
		} else {
			changed := append([]byte(nil), certificate.Certificate[0]...)
			changed[len(changed)-1] ^= 1
			err = os.WriteFile(path, changed, 0600)
		}
		if err != nil {
			t.Fatal(err)
		}
		if allowed, err := monitor.InboundControlAllowed(peer, now); err == nil || allowed {
			t.Fatal("certificate change concealed by monitor", allowed, err)
		}
	}
}
