package handoff

import (
	"context"
	"testing"
	"time"
)

func TestForkAuthorityKeepsLateDiagnosticsOutOfCompletedSample(t *testing.T) {
	parent := &sample{}
	ctx := context.WithValue(context.Background(), sampleKey{}, parent)
	child, merge := ForkAuthority(ctx)
	done := make(chan struct{})
	go func() {
		RecordAuthority(child, AuthorityLock, time.Microsecond)
		RecordAuthority(child, AuthorityPipeOwner, 2*time.Microsecond)
		RecordPipeOwnerCycles(child, 42)
		close(done)
	}()
	// The original can be finalized/read while the native query still writes.
	if parent.vector.AuthorityMeasured != 0 || parent.vector.PipeOwnerCyclesMeasured {
		t.Fatal("native query shared its caller's sample")
	}
	<-done
	if parent.vector.AuthorityMeasured != 0 {
		t.Fatal("late query changed a sample without a successful join")
	}
	merge()
	if parent.vector.AuthorityMeasured != 3 || parent.vector.AuthorityMicros[1] != 2 || parent.vector.PipeOwnerCycles != 42 || !parent.vector.PipeOwnerCyclesMeasured {
		t.Fatal("joined query lost diagnostic detail", parent.vector)
	}
	RecordAuthority(ctx, AuthorityServiceStatus, 3*time.Microsecond)
	merge()
	if parent.vector.AuthorityMicros[2] != 3 {
		t.Fatal("merge replaced a later diagnostic span")
	}
}
