//go:build windows && desktop

package main

import (
	"context"
	"github.com/0disoft/krasis/internal/launcherui"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productlog"
	"github.com/0disoft/krasis/internal/productpanel"
	"github.com/wailsapp/wails/v3/pkg/application"
	"sync"
	"sync/atomic"
	"time"
)

type WorkspaceService struct {
	ctx         context.Context
	control     string
	title       string
	clipGate    sync.Mutex
	clipEpoch   atomic.Uint64
	raw         *productpanel.DesktopBridge
	managed     *productpanel.DesktopLifecycle
	pairs       *productpanel.PairingBridge
	diagnostics *productpanel.DiagnosticsBridge
	done        chan struct{}
	jobs        sync.WaitGroup
}

func (s *WorkspaceService) ServiceStartup(context.Context, application.ServiceOptions) error {
	if s.managed != nil {
		s.done = make(chan struct{})
		go func() { defer close(s.done); s.managed.Run() }()
		s.jobs.Add(1)
		go func() {
			defer s.jobs.Done()
			launcherui.RunClipboardWorker(s.ctx, s.control, s.title, &s.clipGate, &s.clipEpoch)
		}()
	}
	return nil
}
func (s *WorkspaceService) ServiceShutdown() error {
	if s.done != nil {
		<-s.done
	}
	s.jobs.Wait()
	return nil
}
func (s *WorkspaceService) disconnectAsync() {
	s.jobs.Add(1)
	go func() { defer s.jobs.Done(); s.Disconnect() }()
}
func (s *WorkspaceService) Refresh() productpanel.BridgeSnapshot {
	if s.managed != nil {
		return s.managed.Refresh()
	}
	return s.raw.Refresh()
}
func (s *WorkspaceService) Connect(alias, fingerprint string) productpanel.BridgeSnapshot {
	if s.managed != nil {
		return s.managed.Connect(alias, fingerprint)
	}
	return s.raw.Connect(alias, fingerprint)
}
func (s *WorkspaceService) Disconnect() productpanel.BridgeSnapshot {
	if s.managed != nil {
		return s.managed.Disconnect()
	}
	return s.raw.Disconnect()
}
func (s *WorkspaceService) Restart() productpanel.BridgeSnapshot {
	if s.managed != nil {
		return s.managed.Restart()
	}
	return s.raw.Refresh()
}

// Export locally before replacement. Delivery is best-effort and bounded, but
// failure to preserve the local evidence blocks automatic process replacement.
func diagnose(local, control string) func(context.Context, string) error {
	return func(parent context.Context, fingerprint string) error {
		if _, err := productlog.Export(local); err != nil {
			return err
		}
		if len(fingerprint) != 64 {
			return nil
		}
		ctx, cancel := context.WithTimeout(parent, 5*time.Second)
		defer cancel()
		client, err := localapi.OpenClient(control)
		if err != nil {
			return nil
		}
		result, err := client.SendDiagnostic(ctx, fingerprint)
		if err != nil || result.Code != "queued" {
			return nil
		}
		timer := time.NewTicker(250 * time.Millisecond)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-timer.C:
				status, err := client.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "STATUS"})
				if err != nil || status.Status.Diagnostic != "QUEUED" {
					return nil
				}
			}
		}
	}
}
