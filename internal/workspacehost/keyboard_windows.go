//go:build windows && desktop

package workspacehost

import (
	"context"
	"github.com/0disoft/krasis/internal/keyboardstate"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/wininput"
	"github.com/wailsapp/wails/v3/pkg/application"
	"time"
)

type KeyboardSnapshot struct {
	Schema string              `json:"schema"`
	Code   string              `json:"code"`
	Peer   string              `json:"peer"`
	Local  keyboardstate.State `json:"local"`
	Remote keyboardstate.State `json:"remote"`
}

// No queue attachment and no unbounded GUI wait. A timed-out callback may not
// later inject a toggle after the user has switched applications.
func keyboardOnUI[T any](ctx context.Context, operation func() T) (T, bool) {
	done := make(chan T, 1)
	application.InvokeAsync(func() {
		if ctx.Err() == nil {
			done <- operation()
		}
	})
	select {
	case value := <-done:
		return value, ctx.Err() == nil
	case <-ctx.Done():
		var zero T
		return zero, false
	}
}

// Reporting belongs to the native host, not the renderer. A hidden or suspended
// WebView must not stop publishing safe UNKNOWN observations or fresh samples
// after the user brings Krasis forward. This never opens or focuses a window.
func (s *WorkspaceService) runKeyboardReporter() {
	defer s.jobs.Done()
	runKeyboardReports(s.ctx, func(ctx context.Context) (keyboardstate.State, bool) {
		return keyboardOnUI(ctx, func() keyboardstate.State { return wininput.ReadLockState(s.clipboardWindow.Load()) })
	}, func(ctx context.Context, local keyboardstate.State) {
		if client, err := localapi.OpenClient(s.control); err == nil {
			_, _ = client.Keyboard(ctx, localapi.KeyboardRequest{Schema: localapi.KeyboardSchema, Local: local})
		}
	})
}

func runKeyboardReports(ctx context.Context, sample func(context.Context) (keyboardstate.State, bool), report func(context.Context, keyboardstate.State)) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for ctx.Err() == nil {
		attempt, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		if local, ok := sample(attempt); ok && attempt.Err() == nil {
			report(attempt, local)
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (s *WorkspaceService) KeyboardRefresh(peer string) KeyboardSnapshot {
	v := KeyboardSnapshot{Schema: localapi.KeyboardSchema, Code: "unavailable", Peer: peer, Local: keyboardstate.Unknown(), Remote: keyboardstate.Unknown()}
	ctx, cancel := context.WithTimeout(s.ctx, 500*time.Millisecond)
	defer cancel()
	local, ok := keyboardOnUI(ctx, func() keyboardstate.State { return wininput.ReadLockState(s.clipboardWindow.Load()) })
	if !ok {
		return v
	}
	v.Local = local
	client, err := localapi.OpenClient(s.control)
	if err != nil {
		return v
	}
	remote, err := client.Keyboard(ctx, localapi.KeyboardRequest{Schema: localapi.KeyboardSchema, Peer: peer, Local: local})
	if err != nil {
		return v
	}
	v.Code, v.Remote = remote.Code, remote.Remote
	return v
}

func (s *WorkspaceService) KeyboardMatch(peer string) KeyboardSnapshot {
	v := s.KeyboardRefresh(peer)
	if peer == "" || v.Code != "ok" || !v.Remote.LocksKnown() {
		v.Code = "unavailable"
		return v
	}
	ctx, cancel := context.WithTimeout(s.ctx, 250*time.Millisecond)
	defer cancel()
	accepted, ok := keyboardOnUI(ctx, func() bool {
		return wininput.SyncLocalLockState(ctx, s.clipboardWindow.Load(), v.Remote) == nil
	})
	v.Code = "rejected"
	if !ok {
		v.Code = "unavailable"
	} else if accepted {
		v.Code = "sent"
	}
	return v
}
