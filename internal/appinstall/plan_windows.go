//go:build windows

package appinstall

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/0disoft/krasis/internal/securefs"
)

type installPlan struct {
	receipt Receipt
	setup   []byte
	idle    io.Closer
}

// Receipt/history and source-image failures must not close a usable app. Keep
// the validated bytes for publication rather than reopening the source later.
func prepareInstallPlan(ctx context.Context, dir *securefs.Directory, self string, next Receipt, inspect func(Receipt) (Receipt, error), stop func() (io.Closer, error)) (installPlan, error) {
	if err := ctx.Err(); err != nil {
		return installPlan{}, err
	}
	if data, err := read(dir, "installation.json", 8192); err == nil {
		old, err := DecodeReceipt(data)
		if err != nil {
			return installPlan{}, err
		}
		if old.Generation > next.Generation {
			return installPlan{}, ErrOlder
		}
		for _, name := range old.Folders {
			if !slices.Contains(next.Folders, name) {
				next.Folders = append(next.Folders, name)
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return installPlan{}, err
	}
	// The package and old receipt own identity validation; bound merged history.
	if next.Schema != Schema || len(next.Folders) > 64 {
		return installPlan{}, ErrInvalid
	}
	if inspect != nil {
		var err error
		next, err = inspect(next)
		if err != nil {
			return installPlan{}, err
		}
	}
	source, err := securefs.OpenReadDirectory(filepath.Dir(self))
	if err != nil {
		return installPlan{}, err
	}
	data, err := read(source, filepath.Base(self), 64<<20)
	err = errors.Join(err, source.Close())
	if err != nil {
		return installPlan{}, err
	}
	if err := ctx.Err(); err != nil {
		return installPlan{}, err
	}
	plan := installPlan{receipt: next, setup: data}
	if stop != nil {
		plan.idle, err = stop()
		if err != nil {
			if plan.idle != nil {
				err = errors.Join(err, plan.idle.Close())
			}
			return installPlan{}, err
		}
		if plan.idle == nil {
			return installPlan{}, &shutdownError{"agent ownership", ErrBusy}
		}
		if err := ctx.Err(); err != nil {
			return installPlan{}, errors.Join(err, plan.idle.Close())
		}
	}
	return plan, nil
}
