package residentagent

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/wininput"
	"os"
)

// An independent child journals before injection and releases on parent EOF.
type linuxProductGuard struct {
	*crashguard.Process
	closeInput func() error
}

func (g *linuxProductGuard) Close() error { return errors.Join(g.Process.Close(), g.closeInput()) }
func portableProductInput(ctx context.Context, distance, width int32) (*wininput.InputAdapter, productSessionGuard, error) {
	input, closeInput, err := wininput.OpenLinuxInput(ctx, distance, width)
	if err != nil {
		return nil, nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		_ = closeInput()
		return nil, nil, err
	}
	guard, err := crashguard.StartProduct(exe)
	if err != nil {
		return nil, nil, errors.Join(err, closeInput())
	}
	return input, &linuxProductGuard{guard, closeInput}, nil
}
