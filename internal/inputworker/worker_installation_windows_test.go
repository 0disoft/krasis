//go:build windows

package inputworker

import (
	"context"
	"testing"
)

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
	for _, proof := range []*Authority{nil, {}, {kind: authorityAgent, checkInput: func() error { return nil }}, {kind: authorityService, checkInput: func() error { return ErrAuthority }}} {
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
