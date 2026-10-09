//go:build windows && desktop

package workspacehost

import (
	"bytes"
	"context"
	"path/filepath"

	"github.com/0disoft/krasis/internal/launchrecord"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productpanel"
	"github.com/0disoft/krasis/internal/securefs"
	"github.com/0disoft/krasis/internal/updatebundle"
)

type olderUIPlan interface {
	HasProcesses() bool
	Stop(context.Context) error
	Close() error
}

type uiTakeoverOps struct {
	runtime func(context.Context) (localapi.RuntimeInfo, error)
	prepare func() (olderUIPlan, error)
	verify  func() error
}

// The incoming UI owns this path, including upgrades from old UIs that cannot
// hand off after their agent has already changed. No agent mutation is sent.
func takeOverMatchedUI(ctx context.Context, generation int, revision string, ops uiTakeoverOps) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	info, err := ops.runtime(ctx)
	if err != nil {
		return false, nil
	} // Ordinary startup owns agent recovery.
	if info.Protocol != localapi.AgentProtocol || info.Generation != generation || info.Build != revision || info.Instance == "" {
		return false, nil
	}
	plan, err := ops.prepare()
	if err != nil {
		return false, err
	}
	defer plan.Close()
	if !plan.HasProcesses() {
		return false, nil
	}
	if err := ops.verify(); err != nil {
		return false, err
	}
	current, err := ops.runtime(ctx)
	if err != nil {
		return false, err
	}
	if current != info {
		return false, productpanel.ErrAgentIncompatible
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := plan.Stop(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func prepareMatchedUI(ctx context.Context, base, control, self string, generation int, revision string) (bool, error) {
	return takeOverMatchedUI(ctx, generation, revision, uiTakeoverOps{
		runtime: func(ctx context.Context) (localapi.RuntimeInfo, error) {
			client, err := localapi.OpenClient(control)
			if err != nil {
				return localapi.RuntimeInfo{}, err
			}
			return client.Runtime(ctx)
		},
		prepare: func() (olderUIPlan, error) { return launchrecord.PrepareOlderUIShutdown(base, generation) },
		verify: func() error {
			guard, err := securefs.OpenReadDirectory(filepath.Dir(self))
			if err != nil {
				return err
			}
			defer guard.Close()
			archive, err := updatebundle.Pack(guard)
			if err != nil {
				return err
			}
			bundle, err := updatebundle.Read(bytes.NewReader(archive), int64(len(archive)))
			if err != nil {
				return err
			}
			if bundle.Metadata().Generation != generation || bundle.Metadata().Revision != revision {
				return productpanel.ErrAgentIncompatible
			}
			return nil
		},
	})
}
