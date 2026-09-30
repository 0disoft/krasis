//go:build windows

package inputworker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallWriteRejectsUnprotectedFileWithoutTruncating(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user-owned.json")
	data := []byte("existing untrusted file must remain intact")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	file, err := openInstallWrite(path)
	if file != nil {
		_ = file.Close()
	}
	if err != ErrAuthority {
		t.Fatal("user-owned existing object permitted an install write")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, data) {
		t.Fatal("rejected object was changed before authorization")
	}
}

func TestInstallFilesZeroValueAndCancelledWritesCannotMutate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	path := filepath.Join(t.TempDir(), "never-created.json")
	if err := writeInstallFile(ctx, path, []byte("rejected")); err != ErrAuthority || !missingInstallPath(path) {
		t.Fatal("cancelled write created a file")
	}
	var zero installFiles
	journal := installJournal{Schema: "krasis.input.transaction/v1", Phase: installStage, Target: installTestImages("new")}
	if zero.check(context.Background()) != ErrAuthority || zero.save(context.Background(), journal) != ErrAuthority || zero.finish(context.Background()) != ErrAuthority {
		t.Fatal("zero store authorized a filesystem effect")
	}
	if _, err := zero.acquire(context.Background()); err != ErrAuthority {
		t.Fatal("zero store acquired installation ownership")
	}
}

func TestInstallPayloadRejectsMalformedMatchedHashImages(t *testing.T) {
	agent, input := []byte("not an executable"), []byte("neither is this")
	expected := installImages{Agent: sha256.Sum256(agent), Service: sha256.Sum256(input)}
	for _, images := range []installImages{expected, installImages{}, installTestImages("different")} {
		if payload, err := pinnedInstallPayload(agent, input, images); err != ErrAuthority || payload != nil {
			t.Fatal("checksum alone authorized an executable")
		}
	}
	if (*installPayload)(nil).valid() || (&installPayload{}).valid() {
		t.Fatal("empty payload authorized an installation")
	}
}
