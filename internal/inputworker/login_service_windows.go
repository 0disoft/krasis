//go:build windows

package inputworker

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/loginagent"
)

func runInstalledLoginService(ctx context.Context, installation *Installation, ready func() error) (result error) {
	recorder := newLoginRuntimeRecorder(saveLoginRuntime)
	recorder.record("service", "STARTING", nil)
	defer func() {
		state := "STOPPED"
		if result != nil {
			state = "FAILED"
		}
		recorder.record("service", state, result)
	}()
	readConsole := func() (loginagent.Console, error) {
		console, err := loginagent.ReadConsole()
		state := "UNKNOWN"
		switch console.State {
		case loginagent.BeforeLogin:
			state = "BEFORE_LOGIN"
		case loginagent.UserSession:
			state = "USER_SESSION"
		case loginagent.Unavailable:
			state = "UNAVAILABLE"
		}
		recorder.record("console", state, err)
		return console, err
	}
	owner, err := loginagent.New(loginagent.Dependencies{
		Read: readConsole,
		Start: func(lifetime context.Context, session uint32, profile [32]byte) (loginagent.Worker, error) {
			recorder.record("start", "STARTING", nil)
			worker, err := launchLoginConnection(lifetime, installation, session, profile)
			state := "STARTED"
			if err != nil {
				state = "FAILED"
			}
			recorder.record("start", state, err)
			if worker != nil {
				worker = &observedLoginWorker{Worker: worker, recorder: recorder}
			}
			return worker, err
		},
	})
	if err != nil {
		return err
	}
	// Reserve the genuine first normal pipe before publishing readiness. Keep
	// it owned through login-child bootstrap and transfer it to the normal loop.
	pipe, err := makePipe(pipeName)
	if err != nil {
		_ = owner.Close()
		return err
	}
	defer func() { result = errors.Join(result, pipe.Close()) }()
	// Child bootstrap authenticates this exact ready service record, even when
	// no user agent has connected yet.
	if ready == nil || ready() != nil {
		_ = owner.Close()
		return ErrAuthority
	}
	recorder.record("service", "READY", nil)
	gate := &loginServiceGate{owner: owner, console: readConsole, selection: func() (loginagent.Selection, error) {
		now := time.Now().UTC()
		grant, present, err := ReadLoginGrant(now)
		state := "PRESENT"
		if !present {
			state = "MISSING"
		}
		if err != nil {
			state = "INVALID"
		}
		recorder.record("grant", state, err)
		if err != nil || !present {
			return loginagent.Selection{}, err
		}
		return grant.Selection(now)
	}}
	return runLoginService(ctx, gate, func(lifetime context.Context, admit func(context.Context) error) error {
		return runServiceWithSource(lifetime, installation, admit, func(run context.Context) error { return runServiceLoopWithGate(run, installation, nil, admit, pipe) })
	})
}
