package residentagent

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/crashguard"
)

func TestGuardPreparationUsesPressDeadlineBeforeAnyInjection(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=^TestGuardPreparationUnresponsiveChild$")
	command.Env = append(os.Environ(), "KRASIS_GUARD_PREPARE_BUDGET_HELPER=1")
	guard, err := crashguard.StartLocalInputCommand(command, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = guard.Close() })
	var events []string
	input := &guardedProductInput{InputBoundaryPort: safetyInput{events: &events}, guard: guard}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	started := time.Now()
	err = input.Press(ctx, control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30})
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > 500*time.Millisecond || len(events) != 0 {
		t.Fatalf("guard preparation exceeded the input budget or injected: elapsed=%s events=%v err=%v", time.Since(started), events, err)
	}
	select {
	case <-guard.Done():
	case <-time.After(time.Second):
		t.Fatal("expired protocol owner did not retire")
	}
}

func TestGuardPreparationUnresponsiveChild(t *testing.T) {
	if os.Getenv("KRASIS_GUARD_PREPARE_BUDGET_HELPER") != "1" {
		return
	}
	_, _ = io.Copy(io.Discard, os.Stdin) // Consume commands without claiming a journal acknowledgement.
}
