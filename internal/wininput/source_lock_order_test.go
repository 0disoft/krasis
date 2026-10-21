package wininput

import (
	"github.com/0disoft/krasis/internal/control"
	"runtime"
	"testing"
	"time"
)

func TestCaptureStateWaitNeverOwnsPhysicalCallbackGate(t *testing.T) {
	for _, mode := range []string{"activate", "external", "refresh", "pause"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := NewSourceSession(4, &recordingSourceSink{})
			s.started.Store(true)
			s.capturePaused = mode == "external"
			s.stateMu.Lock()
			stateDone, entered := make(chan struct{}), make(chan struct{})
			go func() {
				defer close(stateDone)
				close(entered)
				switch mode {
				case "activate":
					_ = s.Activate()
				case "external":
					s.UseExternalPhysicalCapture()
				case "refresh":
					s.captureRefreshPending()
				case "pause":
					s.PauseCapture()
				}
			}()
			<-entered
			// Mutex waits are not durable synctest waits. Give the state waiter
			// scheduling turns while retaining the transaction for the whole probe.
			for i := 0; i < 100; i++ {
				runtime.Gosched()
			}
			time.Sleep(10 * time.Millisecond)
			physicalDone := make(chan bool, 1)
			go func() {
				physicalDone <- routePhysicalAction(s, nil, control.InputAction{Kind: control.InputKeyDown, Code: 30}, 0)
			}()
			completed, suppressed := false, false
			select {
			case suppressed = <-physicalDone:
				completed = true
			case <-time.After(500 * time.Millisecond):
			}
			s.stateMu.Unlock()
			<-stateDone
			if !completed {
				<-physicalDone
				t.Fatal("state waiter held the physical callback gate")
			}
			if suppressed || s.Active() || s.PhysicalPressCount() != 1 {
				t.Fatal("local press crossed a pending grant or lost its ledger")
			}
		})
	}
}
