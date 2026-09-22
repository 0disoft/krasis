//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
)

func TestShutdownPostFailureIsNotThreadCompletion(t *testing.T) {
	evidence := &sourceShutdownEvidence{}
	nativeErr := errors.New("delivery failed")
	if err := postSourceShutdown(evidence, func() error { return nativeErr }); !errors.Is(err, ErrSourceShutdownPost) {
		t.Fatal(err)
	}
	synctest.Test(t, func(t *testing.T) {
		results := make(chan sourceInputThreadResult, 1)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err, joined := collectSourceInputThreads(ctx, cancel, results, nil, 1, SourceCleanupTimeout)
		combined := errors.Join(err, evidence.snapshot())
		if joined || !errors.Is(combined, ErrSourceShutdownPost) || !errors.Is(combined, errSourceInputJoinTimeout) {
			t.Fatalf("delivery confused with join: %v %v", joined, combined)
		}
	})
}

func TestPreviousHookOwnerHasBoundedAcquisitionWithoutForcedRelease(t *testing.T) {
	release, err := acquireHookSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := acquireHookSession(context.Background()); !errors.Is(err, ErrSourceOwnerUnavailable) {
		t.Fatalf("owner wait: %v", err)
	}
	if len(hookSessionGate) != 0 {
		t.Fatal("live owner forcibly released")
	}
}
