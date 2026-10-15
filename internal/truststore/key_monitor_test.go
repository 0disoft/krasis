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
