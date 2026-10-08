//go:build windows

package appinstall

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
)

func TestInstallEarlyFailureRetainsCauseAndDoesNotStopProduct(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		base := t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())
		if cancelled {
			cancel()
		}
		_, err := install(ctx, base, "unused.exe", Package{}, func() (io.Closer, error) {
			t.Fatal("failed preflight stopped the product")
			return nil, nil
		})
		cancel()
		cause := ErrInvalid
		if cancelled {
			cause = context.Canceled
		}
		if !errors.Is(err, cause) || InstallationStage(err) != "preflight" {
			t.Fatal("lost first failure", err)
		}
		if entries, err := os.ReadDir(base); err != nil || len(entries) != 0 {
			t.Fatal("failed preflight created product state", entries, err)
		}
	}
}
