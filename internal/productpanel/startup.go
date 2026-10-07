package productpanel

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/localapi"
	"io"
	"time"
)

var (
	ErrStartupBusy         = errors.New("agent startup is busy")
	ErrAgentBusy           = errors.New("agent is active; disconnect or finish pairing before upgrade")
	ErrAgentIncompatible   = errors.New("agent protocol is incompatible")
	ErrAgentNotReady       = errors.New("agent did not become ready")
	ErrAgentRecoveryFailed = errors.New("replacement agent is faulted")
)

type RuntimeClient interface {
	Status(context.Context) (localapi.Status, error)
	Runtime(context.Context) (localapi.RuntimeInfo, error)
	StopForUpgrade(context.Context, string, int, string) (localapi.OperationResult, error)
}

type Startup struct {
	Open              func() (RuntimeClient, error)
	Lock              func() (io.Closer, error)
	Available         func() (bool, error)
	Verify            func() error
	Start             func() error
	BeforeStart       func(context.Context) error
	InputRefresh      func(context.Context) error
	InputSetup        func() string
	InputRestoreCheck func() error
	Pause             func(context.Context) error
	Generation        int
	Revision          string
}

// Read-only proof for clearing stale startup failures. A usable status alone
// cannot identify the running build or validate this UI's complete bundle.
func (s Startup) ObservedReady(ctx context.Context) bool {
	if ctx.Err() != nil || s.Open == nil || s.Verify == nil || s.Revision == "" {
		return false
	}
	client, err := s.Open()
	if err != nil || client == nil {
		return false
	}
	info, err := client.Runtime(ctx)
	if err != nil || !s.maintenanceCompatible(info, false) {
		return false
	}
	status, err := client.Status(ctx)
	if err != nil || status.State != "LOCAL" && status.State != "CONNECTED" {
		return false
	}
	return s.Verify() == nil && ctx.Err() == nil
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
				if err := s.start(ctx); err != nil {
					return err
				}
				started, stopping = true, false
			}
		} else {
			client, err := s.Open()
			var status localapi.Status
			if err == nil {
				status, err = client.Status(ctx)
			}
			if err == nil {
				info, err := client.Runtime(ctx)
				// Older product agents implement the same status/control v1 API.
				// Missing lifecycle metadata is not permission to stop an unknown process.
				if errors.Is(err, localapi.ErrNotFound) {
					if s.Revision != "" {
						return ErrAgentIncompatible
					}
					return nil
				}
				if err != nil {
					return err
				}
				if info.Protocol != localapi.AgentProtocol {
					return ErrAgentIncompatible
				}
				faulted := status.State == "FAULTED"
				if info.Generation >= s.Generation && !faulted {
					if s.Revision != "" && (info.Generation != s.Generation || info.Build != s.Revision) {
						return ErrAgentIncompatible
					}
					return nil
				}
				// A fault latch belongs to this process. Never clear it in place or
				// restart repeatedly; replace only after verified process ownership ends.
				if faulted && started {
					return ErrAgentRecoveryFailed
				}
				if faulted && info.Generation > s.Generation {
					return ErrAgentIncompatible
				}
				if started {
					return ErrAgentIncompatible
				}
				if err := s.Verify(); err != nil {
					return err
				}
				result, err := client.StopForUpgrade(ctx, info.Instance, s.Generation, s.Revision)
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
					if err := s.start(ctx); err != nil {
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

func (s Startup) start(ctx context.Context) error {
	if s.BeforeStart != nil {
		if err := s.BeforeStart(ctx); err != nil {
			if errors.Is(err, ErrInputRefreshPending) {
				return &inputMaintenancePending{} // Ensure verified the instance lease before start.
			}
			if !errors.Is(err, ErrInputRefreshCancelled) && !errors.Is(err, ErrInputRefreshFailed) {
				return err
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.Start()
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
	if errors.Is(err, ErrAgentRecoveryFailed) {
		return "새 에이전트에서도 오류가 발생했습니다. 반복 재시작하지 않습니다. 진단을 내보내주세요."
	}
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
