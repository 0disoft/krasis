//go:build windows

package inputworker

import (
	"os"
	"os/exec"
	"reflect"
	"sync"
	"testing"

	"golang.org/x/sys/windows"
)

func TestSourceProcessPriorityScope(t *testing.T) {
	for _, test := range []struct {
		name      string
		initial   uint32
		failSet   int
		external  uint32
		wantSets  []uint32
		wantFinal uint32
	}{
		{"normal", windows.NORMAL_PRIORITY_CLASS, 0, 0, []uint32{windows.ABOVE_NORMAL_PRIORITY_CLASS, windows.NORMAL_PRIORITY_CLASS}, windows.NORMAL_PRIORITY_CLASS},
		{"query-failure", 0, 0, 0, nil, 0},
		{"idle", windows.IDLE_PRIORITY_CLASS, 0, 0, nil, windows.IDLE_PRIORITY_CLASS},
		{"below-normal", windows.BELOW_NORMAL_PRIORITY_CLASS, 0, 0, nil, windows.BELOW_NORMAL_PRIORITY_CLASS},
		{"above-normal", windows.ABOVE_NORMAL_PRIORITY_CLASS, 0, 0, nil, windows.ABOVE_NORMAL_PRIORITY_CLASS},
		{"high", windows.HIGH_PRIORITY_CLASS, 0, 0, nil, windows.HIGH_PRIORITY_CLASS},
		{"realtime", windows.REALTIME_PRIORITY_CLASS, 0, 0, nil, windows.REALTIME_PRIORITY_CLASS},
		{"promotion-denied", windows.NORMAL_PRIORITY_CLASS, 1, 0, []uint32{windows.ABOVE_NORMAL_PRIORITY_CLASS}, windows.NORMAL_PRIORITY_CLASS},
		{"restore-denied", windows.NORMAL_PRIORITY_CLASS, 2, 0, []uint32{windows.ABOVE_NORMAL_PRIORITY_CLASS, windows.NORMAL_PRIORITY_CLASS}, windows.ABOVE_NORMAL_PRIORITY_CLASS},
		{"external-policy-change", windows.NORMAL_PRIORITY_CLASS, 0, windows.BELOW_NORMAL_PRIORITY_CLASS, []uint32{windows.ABOVE_NORMAL_PRIORITY_CLASS}, windows.BELOW_NORMAL_PRIORITY_CLASS},
		{"restore-query-failure", windows.NORMAL_PRIORITY_CLASS, 0, ^uint32(0), []uint32{windows.ABOVE_NORMAL_PRIORITY_CLASS}, windows.ABOVE_NORMAL_PRIORITY_CLASS},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := test.initial
			var sets []uint32
			failQuery := false
			release := sourceProcessPriorityScope(func() uint32 {
				if failQuery {
					return 0
				}
				return current
			}, func(priority uint32) bool {
				sets = append(sets, priority)
				if len(sets) == test.failSet {
					return false
				}
				current = priority
				return true
			})
			if test.external == ^uint32(0) {
				failQuery = true
			} else if test.external != 0 {
				current = test.external
			}
			// Cleanup is idempotent even if several deferred exit paths race.
			var done sync.WaitGroup
			for range 8 {
				done.Go(release)
			}
			done.Wait()
			if current != test.wantFinal || !reflect.DeepEqual(sets, test.wantSets) {
				t.Fatalf("priority=%x sets=%x; want priority=%x sets=%x", current, sets, test.wantFinal, test.wantSets)
			}
		})
	}
}

// Use an owned child so no priority policy changes affect the test runner.
func TestSourceProcessPriorityNative(t *testing.T) {
	const childEnvironment = "KRASIS_SOURCE_PRIORITY_TEST_CHILD"
	if os.Getenv(childEnvironment) != "1" {
		command := exec.Command(os.Args[0], "-test.run=^TestSourceProcessPriorityNative$", "-test.v")
		command.Env = append(os.Environ(), childEnvironment+"=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("priority child: %v\n%s", err, output)
		}
		return
	}
	process := windows.CurrentProcess()
	before, err := windows.GetPriorityClass(process)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetPriorityClass(process, windows.NORMAL_PRIORITY_CLASS); err != nil {
		t.Fatal(err)
	}
	defer windows.SetPriorityClass(process, before)
	release := sourceCaptureProcessSchedulingScope()
	defer release()
	if current, err := windows.GetPriorityClass(process); err != nil || current != windows.ABOVE_NORMAL_PRIORITY_CLASS {
		t.Fatalf("promotion: class=%x err=%v", current, err)
	}
	release()
	if current, err := windows.GetPriorityClass(process); err != nil || current != windows.NORMAL_PRIORITY_CLASS {
		t.Fatalf("restoration: class=%x err=%v", current, err)
	}
}
