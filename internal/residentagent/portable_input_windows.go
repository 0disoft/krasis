//go:build windows

package residentagent

import (
	"context"
	"errors"
	"os"

	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/inputworker"
	"github.com/0disoft/krasis/internal/wininput"
)

func portableProductInput(ctx context.Context, distance, width int32) (*wininput.InputAdapter, productSessionGuard, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, nil, err
	}
	processes, err := crashguard.StartPortableProcesses(executable)
	if err != nil {
		return nil, nil, err // Startup retains its own full retirement receipt.
	}
	client, err := inputworker.NewPortablePairClient(ctx, processes)
	if err != nil {
		return nil, nil, errors.Join(err, processes.Close())
	}
	input, err := wininput.NewInputAdapterWithInjection(distance, width, client)
	if err != nil {
		return nil, nil, errors.Join(err, processes.Close())
	}
	return input, processes, nil
}
