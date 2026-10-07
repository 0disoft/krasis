package residentagent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

func TestPrivilegedAdmissionExpiresWhilePreparationOwnsLock(t *testing.T) {
	for _, action := range []string{"input", "open", "prepare"} {
		t.Run(action, func(t *testing.T) {
			client := &testPrivilegedClient{}
			input := &privilegedLeaseInput{client: client, expires: time.Now().Add(time.Hour)}
			input.mu.Lock()
			defer input.mu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				switch action {
				case "input":
					done <- input.Press(ctx, control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30})
				case "open":
					done <- input.open(ctx, application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}, time.Now())
				case "prepare":
					_, err := input.prepareIdle(ctx)
					done <- err
				}
			}()
			select {
			case err := <-done:
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatal(err)
				}
				if action == "input" {
					if certainty, ok := application.InjectionCertaintyOf(err); !ok || certainty != application.InjectionNotApplied {
						t.Fatal("not-sent certainty missing")
					}
				}
			case <-time.After(time.Second):
				t.Fatal("canceled request waited for preparation")
			}
			if client.presses.Load() != 0 || client.closes.Load() != 0 {
				t.Fatal("canceled admission changed active client")
			}
		})
	}
}
