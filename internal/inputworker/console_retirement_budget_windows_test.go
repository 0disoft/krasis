//go:build windows

package inputworker

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"golang.org/x/sys/windows"
)

func TestConsoleRetirementSharesOneWaitBudget(t *testing.T) {
	for _, tc := range []struct {
		name     string
		pair     bool
		budget   uint32
		drain    time.Duration
		jobClose time.Duration
		wantWait uint32
		wantTime time.Duration
	}{
		{"pair_drains", true, 50, 30 * time.Millisecond, 0, 20, 50 * time.Millisecond},
		{"job_close_consumes_budget", true, 50, 0, 70 * time.Millisecond, 0, 70 * time.Millisecond},
		{"ordinary_job", false, 50, 0, 15 * time.Millisecond, 35, 50 * time.Millisecond},
		{"nonblocking_reap", true, 0, 0, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				started := time.Now()
				var waited uint32
				var jobs, processes, terminations int
				w := &ConsoleWorker{job: 11, process: 22, pairJob: tc.pair,
					nativeChildPossible: true, nativeChildRetired: true, done: make(chan struct{})}
				w.closeOps = &consoleCloseOps{
					terminateJob: func(windows.Handle, uint32) error { terminations++; return nil },
					activeProcesses: func(windows.Handle) (uint32, error) {
						if time.Since(started) < tc.drain {
							return 1, nil
						}
						return 0, nil
					},
					closeHandle: func(h windows.Handle) error {
						switch h {
						case 11:
							jobs++
							time.Sleep(tc.jobClose)
						case 22:
							processes++
						default:
							t.Error("closed an unowned handle")
						}
						return nil
					},
					wait: func(_ windows.Handle, milliseconds uint32) (uint32, error) {
						waited = milliseconds
						time.Sleep(time.Duration(milliseconds) * time.Millisecond)
						return uint32(windows.WAIT_TIMEOUT), nil
					},
				}
				if err := w.tryRetire(tc.budget); !errors.Is(err, ErrAuthority) {
					t.Fatal("unconfirmed process accepted", err)
				}
				if elapsed := time.Since(started); waited != tc.wantWait || elapsed != tc.wantTime {
					t.Fatalf("renewed retirement budget: processWait=%dms, elapsed=%s; want %dms and %s", waited, elapsed, tc.wantWait, tc.wantTime)
				}
				if jobs != 1 || processes != 0 || w.process != 22 || w.job != 0 || w.closed || w.poisoned {
					t.Fatal("timeout lost or released exact process ownership")
				}
				select {
				case <-w.done:
					t.Fatal("timeout published retirement")
				default:
				}
				w.closeOps.wait = func(_ windows.Handle, milliseconds uint32) (uint32, error) {
					if milliseconds != 0 {
						t.Error("reaper attempted a blocking wait")
					}
					return windows.WAIT_OBJECT_0, nil
				}
				if err := w.tryRetire(0); err != nil || !w.closed || w.process != 0 || jobs != 1 || processes != 1 {
					t.Fatal("late exact exit did not complete once", err)
				}
				if tc.pair && terminations != 1 || !tc.pair && terminations != 0 {
					t.Fatal("job termination was repeated or changed")
				}
			})
		})
	}
}
