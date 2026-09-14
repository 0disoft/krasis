package productpanel

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/localapi"
	"io"
	"time"
)

var (
	ErrStartupBusy       = errors.New("agent startup is busy")
	ErrAgentBusy         = errors.New("agent is active; disconnect or finish pairing before upgrade")
	ErrAgentIncompatible = errors.New("agent protocol is incompatible")
	ErrAgentNotReady     = errors.New("agent did not become ready")
)

type RuntimeClient interface {
	Status(context.Context) (localapi.Status, error)
	Runtime(context.Context) (localapi.RuntimeInfo, error)
	StopForUpgrade(context.Context, string) (localapi.OperationResult, error)
}

type Startup struct {
	Open       func() (RuntimeClient, error)
	Lock       func() (io.Closer, error)
	Available  func() (bool, error)
	Verify     func() error
	Start      func() error
	Pause      func(context.Context) error
	Generation int
}

// Ensure is a bounded, single-start operation. Timers in the GUI do not invoke it.
func (s Startup) Ensure(ctx context.Context) error {
	lease, err := s.Lock()
	if err != nil {
		return ErrStartupBusy
	}
	defer lease.Close()
	started, stopping := false, false
	for ctx.Err() == nil {
		if stopping {
			available, err := s.Available()
			if err != nil {
				return err
			}
			if available {
				if err := s.Verify(); err != nil {
					return err
				}
				if err := s.Start(); err != nil {
					return err
				}
				started, stopping = true, false
			}
		} else {
			client, err := s.Open()
			if err == nil {
				_, err = client.Status(ctx)
			}
			if err == nil {
				info, err := client.Runtime(ctx)
				// Older product agents implement the same status/control v1 API.
				// Missing lifecycle metadata is not permission to stop an unknown process.
				if errors.Is(err, localapi.ErrNotFound) {
					return nil
				}
				if err != nil {
					return err
				}
				if info.Protocol != localapi.AgentProtocol {
					return ErrAgentIncompatible
				}
				if info.Generation >= s.Generation {
					return nil
				}
				if started {
					return ErrAgentIncompatible
				}
				if err := s.Verify(); err != nil {
					return err
				}
				result, err := client.StopForUpgrade(ctx, info.Instance)
				if err != nil {
					return err
				}
				if result.Status != "COMPLETED" || result.Code != "upgrade_stop_requested" {
					return ErrAgentBusy
				}
				stopping = true
			} else if !errors.Is(err, localapi.ErrUnavailable) && !errors.Is(err, localapi.ErrTransport) {
				return err
			} else if !started {
				available, err := s.Available()
				if err != nil {
					return err
				}
				if available {
					if err := s.Verify(); err != nil {
						return err
					}
					if err := s.Start(); err != nil {
						return err
					}
					started = true
				}
			}
		}
		if err := s.Pause(ctx); err != nil {
			return ErrAgentNotReady
		}
	}
	return ErrAgentNotReady
}

func startupPause(ctx context.Context) error {
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func StartupNotice(err error) string {
	if errors.Is(err, ErrAgentBusy) {
		return "연결 또는 페어링 중인 에이전트는 교체하지 않습니다. 작업 종료 후 새로고침하세요."
	}
	if errors.Is(err, ErrStartupBusy) {
		return "다른 창에서 에이전트를 준비 중입니다. 잠시 후 새로고침하세요."
	}
	if errors.Is(err, ErrAgentIncompatible) {
		return "에이전트 버전이 호환되지 않습니다. 같은 배포 묶음을 사용하세요."
	}
	return "에이전트 준비에 실패했습니다. ZIP 전체를 새 폴더에 풀었는지 확인하고 새로고침하세요."
}
