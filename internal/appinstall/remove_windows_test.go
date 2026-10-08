//go:build windows

package appinstall

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCancelledRemovalDoesNotCreateOrChangeProductState(t *testing.T) {
	base := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Remove(ctx, base); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled removal continued", err)
	}
	if _, err := os.Stat(filepath.Join(base, "Krasis")); !os.IsNotExist(err) {
		t.Fatal("cancelled removal created product state", err)
	}
}
