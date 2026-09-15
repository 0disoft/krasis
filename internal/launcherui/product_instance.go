package launcherui

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/0disoft/krasis/internal/agentinstance"
)

const productWindowClass = "KrasisProductControlWindow"

// A nil lease means an existing window handled the launch. Nothing after this
// gate may open logs, prepare an agent or create another management window.
func claimProductWindow(ctx context.Context, acquire func() (io.Closer, error), showExisting func() bool) (io.Closer, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		lease, err := acquire()
		if err == nil {
			// Older GUIs did not acquire the lifetime mutex. Reuse their
			// window too, but never terminate them or alter their session.
			if showExisting() {
				return nil, lease.Close()
			}
			return lease, nil
		}
		if !errors.Is(err, agentinstance.ErrAlreadyRunning) {
			return nil, err
		}
		if showExisting() {
			return nil, nil
		}
		// The owner may still be creating its window or shutting down.
		// Retry ownership as well as discovery, bounded by the caller.
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func controlLogWarning(available, failed bool, openErr error) string {
	if !available {
		if errors.Is(openErr, agentinstance.ErrAlreadyRunning) {
			return "다른 관리 앱이 로그를 사용 중입니다. 트레이에서 기존 관리 앱을 종료한 뒤 다시 실행하세요."
		}
		return "관리 앱 로그 파일을 열지 못했습니다. 저장 권한과 디스크 상태를 확인하세요."
	}
	if failed {
		return "관리 앱 로그 일부를 저장하지 못했거나 기록이 누락됐습니다."
	}
	return ""
}
