//go:build windows

package inputworker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkerImageAdoptionChecksLiveServiceAfterVerification(t *testing.T) {
	for _, mode := range []string{"valid", "service_restarted", "proof_closed", "image_changed"} {
		t.Run(mode, func(t *testing.T) {
			installation := pendingWorkerFixture(t)
			file, err := os.Open(installation.images[1].path)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close() // Caller still owns it on every rejected adoption.
			verified, current, checks := false, true, 0
			proof := &Authority{kind: authorityService, checkRecovery: func() error { return nil }, closeHandles: func() error { return nil }}
			proof.checkInput = func() error {
				checks++
				if !verified {
					t.Fatal("SCM queried before file verification instead of at commit")
				}
				if !current {
					return ErrAuthority
				}
				return nil
			}
			err = installation.adoptWorkerAgentImageChecked(proof, file, func(path string) bool {
				if path != filepath.Join(filepath.Dir(installation.images[1].path), "krasis-agent.exe") {
					t.Fatal("adoption no longer selects the fixed protected name")
				}
				verified = true
				current = mode != "service_restarted"
				if mode == "proof_closed" {
					_ = proof.Close()
				}
				return mode != "image_changed"
			})
			if mode == "valid" {
				if err != nil || checks != 1 || installation.workerPending || installation.images[0].file != file || len(installation.files) != 5 {
					t.Fatal("valid transfer did not commit with one fresh query", err, checks)
				}
			} else if err != ErrAuthority || !installation.workerPending || len(installation.files) != 4 || installation.images[0].file != nil {
				t.Fatal("failed final verification published an image", err)
			}
			if _, err := file.Stat(); err != nil {
				t.Fatal("adoption closed the caller's image", err)
			}
		})
	}
}

func pendingWorkerFixture(t *testing.T) *Installation {
	t.Helper()
	installation := imagePinFixture(t)
	installation.agentDigest = installation.images[0].digest
	installation.serviceDigest = installation.images[1].digest
	if err := installation.images[0].file.Close(); err != nil {
		t.Fatal(err)
	}
	installation.images[0] = installationImage{}
	installation.files = append(installation.files[:3], installation.files[4])
	installation.workerPending = true
	return installation
}

func TestPendingWorkerCannotAuthorizeServiceOrAgentImage(t *testing.T) {
	installation := pendingWorkerFixture(t)
	if _, _, err := installation.digests(); err != ErrAuthority || RunService(installation) != ErrAuthority {
		t.Fatal("partial worker installation authorized service")
	}
	if agent, service, err := installation.workerDigests(); err != nil || agent != installation.agentDigest || service != installation.serviceDigest {
		t.Fatal("worker lost protected metadata")
	}
	service := installation.images[1]
	if pin, err := installation.duplicateImage(service.path, service.digest); err != nil {
		t.Fatal("independent service image unavailable")
	} else {
		_ = pin.Close()
	}
	if pin, err := installation.duplicateImage("agent-image", installation.agentDigest); pin != nil || err != ErrAuthority {
		t.Fatal("pending worker authorized agent")
	}
	if err := installation.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := installation.workerDigests(); err != ErrAuthority || RunInstalledWorker(context.Background(), installation) != ErrAuthority {
		t.Fatal("closed worker bootstrap accepted")
	}
}

func TestWorkerImageRejectsMissingWrongOrRetiredServiceProof(t *testing.T) {
	installation := pendingWorkerFixture(t)
	file := installation.images[1].file
	wrongKind := &Authority{kind: authorityAgent, checkInput: func() error { return nil }, checkRecovery: func() error { return nil }, closeHandles: func() error { return nil }}
	retired := &Authority{kind: authorityService, checkInput: func() error { return ErrAuthority }, checkRecovery: func() error { return nil }, closeHandles: func() error { return nil }}
	closed := &Authority{kind: authorityService, checkInput: func() error { return nil }, checkRecovery: func() error { return nil }, closeHandles: func() error { return nil }}
	if closed.Close() != nil {
		t.Fatal("service proof close failed")
	}
	for _, proof := range []*Authority{nil, {}, wrongKind, retired, closed} {
		if err := installation.adoptWorkerAgentImage(proof, file); err != ErrAuthority {
			t.Fatal("unauthenticated image adopted")
		}
	}
	if !installation.workerPending || len(installation.files) != 4 {
		t.Fatal("failed adoption changed installation")
	}
}

func TestTransferredImageMustMatchExactLiveFile(t *testing.T) {
	installation := imagePinFixture(t)
	image := installation.images[0]
	duplicate, err := installation.duplicateImage(image.path, image.digest)
	if err != nil {
		t.Fatal(err)
	}
	defer duplicate.Close()
	if !samePinnedImage(duplicate, image.file) || samePinnedImage(duplicate, installation.images[1].file) || samePinnedImage(nil, image.file) {
		t.Fatal("file identity check confused different objects")
	}
	_ = image.file.Close()
	if samePinnedImage(duplicate, image.file) {
		t.Fatal("closed proof accepted")
	}
}
