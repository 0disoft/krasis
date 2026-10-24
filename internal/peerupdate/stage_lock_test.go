package peerupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

type preparedTestBundle struct {
	fakeBundle
	prepare func() error
	publish func() error
}

func (b *preparedTestBundle) PrepareStage(*os.Root, string) (func() error, error) {
	if err := b.prepare(); err != nil {
		return nil, err
	}
	return func() error {
		if err := b.publish(); err != nil {
			return err
		}
		b.staged = true
		return nil
	}, nil
}

func TestReceiveStagePreparationDoesNotHoldManagerLock(t *testing.T) {
	for _, mode := range []string{"success", "revoked", "disconnected", "closed", "prepare failure", "publish failure"} {
		t.Run(mode, func(t *testing.T) {
			m := testManager(t)
			peer := strings.Repeat("a", 64)
			if err := m.Permit(peer, true); err != nil {
				t.Fatal(err)
			}
			failure := errors.New("injected stage failure")
			connected, prepared, published := true, 0, 0
			bundle := &preparedTestBundle{fakeBundle: fakeBundle{generation: 22}}
			bundle.prepare = func() error {
				prepared++
				if !m.mu.TryLock() {
					return errors.New("stage verification holds manager lock")
				}
				m.mu.Unlock()
				if m.Status().State != "VERIFYING" {
					return errors.New("verification lost receive ownership")
				}
				switch mode {
				case "revoked":
					return m.Permit(peer, false)
				case "disconnected":
					connected = false
				case "closed":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					if err := m.closeContext(ctx); !errors.Is(err, ErrBusy) {
						return errors.New("shutdown did not retain the active receiver")
					}
				case "prepare failure":
					return failure
				}
				return nil
			}
			bundle.publish = func() error {
				published++
				if m.mu.TryLock() {
					m.mu.Unlock()
					return errors.New("publication lost permission lock")
				}
				if mode == "publish failure" {
					return failure
				}
				return nil
			}
			m.validate = func(io.ReaderAt, int64) (stagedBundle, error) { return bundle, nil }
			data := []byte("bounded archive fixture")
			err := m.receive(peer, strings.Repeat("b", 32), fmt.Sprintf("%x", sha256.Sum256(data)), bytes.NewReader(data), func() bool { return connected })
			if mode == "success" && err != nil {
				t.Fatal(err)
			}
			if (mode == "revoked" || mode == "disconnected" || mode == "closed") && !errors.Is(err, ErrDenied) {
				t.Fatal("authority changed during verification but publication was admitted", err)
			}
			if strings.HasSuffix(mode, "failure") && !errors.Is(err, failure) {
				t.Fatal("stage failure lost", err)
			}
			wantPublished := 0
			if mode == "success" || mode == "publish failure" {
				wantPublished = 1
			}
			if prepared != 1 || published != wantPublished || bundle.staged != (mode == "success") || bundle.closed != 1 {
				t.Fatalf("stage ownership: prepared=%d published=%d staged=%t closed=%d", prepared, published, bundle.staged, bundle.closed)
			}
			if state := m.Status().State; (state == "STAGED") != (mode == "success") {
				t.Fatal("failed preparation published staged state", state)
			}
		})
	}
}
