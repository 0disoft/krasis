//go:build windows

package launcherui

import (
	"context"
	"os"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productlog"
	"github.com/0disoft/krasis/internal/productpanel"
)

func (a *productWindow) executeAutomaticRecovery(ctx context.Context, action, alias, fingerprint string) productpanel.View {
	open := func() (productpanel.Client, error) { return localapi.OpenClient(a.control) }
	// Preserve evidence before replacing the process. Remote failure must not
	// destroy the local export or prevent safe local recovery.
	path, err := productlog.Export(os.Getenv("LOCALAPPDATA"))
	if err != nil {
		view := productpanel.Execute(ctx, open, "refresh", "")
		view.Notice = "자동 진단 저장에 실패하여 에이전트 교체를 보류했습니다. 저장 공간과 폴더 권한을 확인하세요."
		return view
	}
	a.sendAutomaticDiagnostic(ctx, fingerprint, true)
	if action == "auto_recover" {
		if err := a.startup.Ensure(ctx); err != nil {
			view := productpanel.Execute(ctx, open, "refresh", "")
			view.Notice = productpanel.StartupNotice(err)
			view.DiagnosticPath = path
			return view
		}
		// The alias must still refer to the exact previously connected identity.
		view := productpanel.Execute(ctx, open, "refresh", "")
		if view.State != "LOCAL" || view.Fingerprints[alias] != fingerprint {
			view.Notice = "자동 복구할 기기 또는 안전한 대기 상태를 확인하지 못해 연결을 중단했습니다."
			view.DiagnosticPath = path
			return view
		}
		view = productpanel.Execute(ctx, open, "connect", alias)
		// Queued offers lived in the old agent; queue again after replacement.
		a.sendAutomaticDiagnostic(ctx, fingerprint, false)
		view.DiagnosticPath = path
		return view
	}
	view := productpanel.Execute(ctx, open, "refresh", "")
	view.DiagnosticPath = path
	return view
}

func (a *productWindow) sendAutomaticDiagnostic(parent context.Context, fingerprint string, await bool) {
	if len(fingerprint) != 64 {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	client, err := localapi.OpenClient(a.control)
	if err != nil {
		return
	}
	result, err := client.SendDiagnostic(ctx, fingerprint)
	if err != nil || result.Code != "queued" || !await {
		return
	}
	for ctx.Err() == nil {
		status, err := client.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "STATUS"})
		if err != nil || status.Status.Diagnostic != "QUEUED" {
			return
		}
		if agent.Wait(ctx, 250*time.Millisecond) != nil {
			return
		}
	}
}
