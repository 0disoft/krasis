//go:build windows

package inputworker

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/crashguard"
)

func adoptPortableFixtureOwnership(args []string) (io.Closer, error) {
	if len(args) == 0 {
		return nil, nil
	}
	if len(args) != 1 {
		return nil, ErrProtocol
	}
	return crashguard.AdoptPortableOwnership(args[0])
}

func servePortableFixtureGuardian(args []string, release application.ReleasePort) error {
	if len(args) != 2 && len(args) != 3 {
		return ErrProtocol
	}
	ownership, err := adoptPortableFixtureOwnership(args[2:])
	if err != nil {
		return err
	}
	if ownership != nil {
		defer ownership.Close()
	}
	parent, err := strconv.ParseUint(args[0], 10, strconv.IntSize)
	if err != nil {
		return err
	}
	input, err := strconv.ParseUint(args[1], 10, strconv.IntSize)
	if err != nil {
		return err
	}
	_, err = crashguard.ServePortableGuardian(uintptr(parent), uintptr(input), os.Stdin, os.Stdout, release)
	return err
}

func TestPortablePairRetainsIndependentRecoveryAfterInputFailure(t *testing.T) {
	// This fixture deliberately exercises the product's fixed ownership fence.
	// A running portable session must retire through safe-stop before testing;
	// do not bypass its mutex, skip recovery checks, or kill its children.
	if err := crashguard.CheckPortableRecovery(); err != nil {
		t.Fatalf("portable recovery fixture requires no live portable input owner; use Krasis safe-stop and confirm process exit before this suite: %v", err)
	}
	for _, mode := range []string{"lost_ack", "partial", "hang"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("KRASIS_PORTABLE_RPC_FIXTURE", mode)
			marker := filepath.Join(t.TempDir(), "independent-release")
			t.Setenv("KRASIS_PORTABLE_RELEASE_MARKER", marker)
			processes, err := crashguard.StartPortableProcesses(os.Args[0])
			if err != nil {
				t.Fatal(err)
			}
			defer processes.Close()
			client, err := NewPortablePairClient(context.Background(), processes)
			if err != nil {
				t.Fatal(err)
			}
			key := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}
			if _, err := processes.PreparePress(key); err != nil {
				t.Fatal(err)
			}
			if err := client.Press(context.Background(), key); err != nil {
				t.Fatal(err)
			}
			want := application.InjectionOutcomeUnknown
			if mode == "partial" {
				want = application.InjectionPartiallyApplied
				err = client.Wheel(context.Background(), 120, 120)
			} else {
				err = client.Move(context.Background(), 1, 0)
			}
			if certainty, ok := application.InjectionCertaintyOf(err); !ok || certainty != want {
				t.Fatal("pair lost the original injection certainty", certainty, err)
			}
			if client.Move(context.Background(), 2, 0) == nil {
				t.Fatal("failed pair replayed input")
			}
			closeErr := client.Close()
			var pending *crashguard.ExitPending
			if errors.As(closeErr, &pending) {
				t.Fatal("fixture did not fully retire", closeErr)
			}
			if data, err := os.ReadFile(marker); err != nil || string(data) != "released" {
				t.Fatal("input failure destroyed the independent recovery journal", string(data), err, closeErr)
			}
		})
	}
}
