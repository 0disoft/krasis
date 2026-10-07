//go:build windows && desktop

package workspacehost

import "github.com/wailsapp/wails/v3/pkg/application"

func trayMenuLabels(locale string) ([4]string, bool) {
	switch locale {
	case "ko":
		return [4]string{"열기", "연결 해제", "PIN 창 입력 시험", "관리 앱 종료 · 에이전트 유지"}, true
	case "en":
		return [4]string{"Open", "Disconnect", "PIN input test", "Quit UI · Keep agent running"}, true
	}
	return [4]string{}, false
}

// Only the supported locale crosses the bridge; menu text and actions remain
// native-owned. The existing UI locale preference is the single source of truth.
func (s *WorkspaceService) SetTrayLocale(locale string) bool {
	labels, ok := trayMenuLabels(locale)
	if !ok || s.updateTrayLabels == nil {
		return false
	}
	application.InvokeSync(func() { s.updateTrayLabels(labels) })
	return true
}
