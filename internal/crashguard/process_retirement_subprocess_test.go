package crashguard

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestGuardRetirementNativeHelper(t *testing.T) {
	if os.Getenv("KRASIS_GUARD_RETIREMENT_NATIVE_CHILD") == "1" {
		time.Sleep(time.Minute)
	}
}

func TestNativeGuardRetirementJoinsDelayedKillAndRealExit(t *testing.T) {
	previous := processIPCTimeout
	processIPCTimeout = 50 * time.Millisecond
	defer func() { processIPCTimeout = previous }()
	command := exec.Command(os.Args[0], "-test.run=^TestGuardRetirementNativeHelper$")
	command.Env = append(os.Environ(), "KRASIS_GUARD_RETIREMENT_NATIVE_CHILD=1")
	p, err := startCommand(command)
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	var once sync.Once
	p.kill = func() error { <-release; return p.process.Kill() }
	defer func() {
		once.Do(func() { close(release) })
		<-p.requestAbort()
		select {
		case <-p.Done():
		case <-time.After(2 * time.Second):
			t.Error("native guard child did not exit")
		}
		_ = p.Close()
	}()
	result := make(chan error, 1)
	go func() {
		_, err := p.PreparePress(control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30})
		result <- err
	}()
	select {
	case err := <-result:
		if !errors.Is(err, ErrIPCTimeout) {
			t.Fatal("native helper did not produce the protocol timeout", err)
		}
	case <-time.After(time.Second):
		t.Fatal("protocol caller waited for the delayed native Kill")
	}
	var pending *ExitPending
	if err := p.Close(); !errors.As(err, &pending) {
		t.Fatal("live native child was reported retired", err)
	}
	select {
	case <-p.Done():
		t.Fatal("delayed termination fabricated native child exit")
	case <-pending.Done:
		t.Fatal("delayed native cleanup was reported joined")
	default:
	}
	once.Do(func() { close(release) })
	select {
	case <-pending.Done:
	case <-time.After(2 * time.Second):
		t.Fatal("real native termination did not finish retirement")
	}
	select {
	case <-p.Done():
	default:
		t.Fatal("retirement completed before the exact child was reaped")
	}
}
