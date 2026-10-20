package wininput

import (
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestNativeActivationInspectionDoesNotBlockPhysicalHook(t *testing.T) {
	for _, change := range []string{"none", "key-held", "key-roundtrip", "button-roundtrip", "owner-refresh", "retire"} {
		t.Run(change, func(t *testing.T) {
			session, _ := NewSourceSession(4, &recordingSourceSink{})
			session.started.Store(true)
			entered, release := make(chan struct{}), make(chan struct{})
			session.physicalCheck = func() bool { close(entered); <-release; return false }
			activation := make(chan error, 1)
			go func() { activation <- session.Activate() }()
			<-entered
			local := make(chan bool, 1)
			go func() {
				unsuppressed := true
				switch change {
				case "key-held", "key-roundtrip", "button-roundtrip":
					down, up := control.InputKeyDown, control.InputKeyUp
					if change == "button-roundtrip" {
						down, up = control.InputButtonDown, control.InputButtonUp
					}
					unsuppressed = !routePhysicalAction(session, nil, control.InputAction{Kind: down, Code: 1}, 0)
					if change != "key-held" {
						unsuppressed = !routePhysicalAction(session, nil, control.InputAction{Kind: up, Code: 1}, 0) && unsuppressed
					}
				case "owner-refresh":
					unsuppressed = session.PauseCapture()
					session.ResumeCapture()
				case "retire":
					session.Retire()
				}
				local <- unsuppressed
			}()
			var completed, safe bool
			select {
			case safe = <-local:
				completed = true
			case <-time.After(500 * time.Millisecond):
			}
			close(release)
			err := <-activation
			if !completed {
				<-local
				t.Fatal("native activation query blocked the physical hook/owner")
			}
			if !safe || len(session.queue) != 0 {
				t.Fatal("local input became remote during inspection")
			}
			if change == "none" {
				if err != nil || !session.Active() {
					t.Fatal("stable native inspection rejected", err)
				}
			} else if err == nil || session.Active() {
				t.Fatal("stale inspection activated capture", change, err)
			}
		})
	}
}

func TestPhysicalRevisionSaturatesWithoutReauthorizingOldInspection(t *testing.T) {
	session, _ := NewSourceSession(1, &recordingSourceSink{})
	session.started.Store(true)
	session.physicalRevision = ^uint64(0)
	session.advancePhysicalRevision()
	if session.physicalRevision != ^uint64(0) || session.Activate() == nil || session.Active() {
		t.Fatal("physical revision overflow granted capture")
	}
}
