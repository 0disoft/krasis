package crashguard

import (
	"context"
	"io"
	"os"
	"os/exec"
	"testing"
)

func TestPreparedLocalInputNativeHelper(t *testing.T) {
	if os.Getenv("KRASIS_PREPARED_NATIVE_HELPER") != "1" {
		return
	}
	var bootstrap [4]byte
	if _, err := io.ReadFull(os.Stdin, bootstrap[:]); err != nil || string(bootstrap[:]) != "BIND" {
		os.Exit(3)
	}
	if err := Serve(os.Stdin, os.Stdout, &recordingRelease{}); err != nil {
		os.Exit(4)
	}
}

func TestPreparedLocalInputNativeBindAndDiscard(t *testing.T) {
	for _, bind := range []bool{false, true} {
		t.Run(map[bool]string{false: "discard", true: "bind"}[bind], func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestPreparedLocalInputNativeHelper$")
			command.Env = append(os.Environ(), "KRASIS_PREPARED_NATIVE_HELPER=1")
			prepared, err := PrepareLocalInputCommandContext(context.Background(), command)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = prepared.Close() })
			owner := prepared.process
			if bind {
				bound, err := prepared.Bind(context.Background(), []byte("BIND"))
				if err != nil || bound != owner {
					t.Fatal("native bootstrap failed", err)
				}
				if err := owner.Close(); err != nil {
					t.Fatal("bound native child did not disarm", err)
				}
			}
			if err := prepared.Close(); err != nil {
				t.Fatal("unbound native child did not retire", err)
			}
			select {
			case <-owner.Done():
			default:
				t.Fatal("native child survived cleanup")
			}
			select {
			case <-owner.closeDone:
			default:
				t.Fatal("native cleanup was not joined")
			}
		})
	}
}
