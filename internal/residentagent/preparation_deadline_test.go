package residentagent

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/agent"
)

// A canceled preparation may still be joining its native owner. Its done
// signal is a cleanup receipt, not a timer; entry must bound its optional wait
// independently without manufacturing that receipt or removing the owner.
func TestEntryPreparationWaitRespectsRemainingDeadlineDuringCleanup(t *testing.T) {
	for _, remaining := range []time.Duration{50 * time.Millisecond, agent.InputPreparationEntryWait, 600 * time.Millisecond} {
		t.Run(remaining.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				canceled := false
				p := &preparationCancellation{
					deadline: time.Now().Add(remaining),
					done:     make(chan struct{}), // Native cleanup has not completed.
					cancel:   func() { canceled = true },
				}
				input := &privilegedLeaseInput{}
				input.preparationCancel.Store(p)
				started := time.Now()
				if input.finishPreparationForEntry(ctx) {
					t.Fatal("unfinished preparation was reported as joined")
				}
				if elapsed, expected := time.Since(started), min(remaining, agent.InputPreparationEntryWait); elapsed != expected {
					t.Fatalf("optional wait exceeded its budget: got %s, want %s", elapsed, expected)
				}
				if !canceled || ctx.Err() != nil || input.preparationCancel.Load() != p {
					t.Fatal("entry used its caller timeout or discarded the cleanup owner")
				}
				select {
				case <-p.done:
					t.Fatal("timeout fabricated native cleanup completion")
				default:
				}
			})
		})
	}
}
