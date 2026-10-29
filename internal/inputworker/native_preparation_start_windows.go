//go:build windows

package inputworker

import (
	"context"
	"errors"
)

type nativePairPreparation struct {
	done chan struct{}
	pair *preparedNativePair
	err  error
}

// Called only after the genuine service's fixed native preparation permit.
// Construction has no lease or authority handles; overlap it with the existing
// authenticated bootstrap. Bind joins it before transferring any input rights.
func startNativePairPreparation(ctx context.Context, prepare func(context.Context) (*preparedNativePair, error)) (*preparedNativePair, error) {
	if ctx == nil || ctx.Err() != nil || prepare == nil {
		return nil, ErrAuthority
	}
	task := &nativePairPreparation{done: make(chan struct{})}
	go func() {
		defer close(task.done)
		task.pair, task.err = prepare(ctx)
		task.err = errors.Join(task.err, ctx.Err())
		if task.pair == nil && task.err == nil {
			task.err = ErrAuthority
		}
	}()
	return &preparedNativePair{preparation: task}, nil
}

func (p *preparedNativePair) awaitPreparation(ctx context.Context) (*preparedNativePair, error) {
	if p == nil || ctx == nil {
		return nil, ErrAuthority
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(ErrAuthority, err)
	}
	if task := p.preparation; task != nil {
		select {
		case <-task.done:
			if err := ctx.Err(); err != nil {
				return nil, errors.Join(ErrAuthority, err)
			}
			return task.pair, task.err
		case <-ctx.Done():
			return nil, errors.Join(ErrAuthority, ctx.Err())
		}
	}
	return p, nil
}

// Cancellation denies binding, but cannot certify native retirement. A late
// constructor still owns its exact processes; join it before the final receipt
// or releasing installation pins. No replacement construction is started here.
func (task *nativePairPreparation) close() error {
	<-task.done
	err := task.pair.Close()
	// prepareNativePair already retires all partial owners on failure. Its
	// ordinary construction rejection is not failed cleanup; actual retirement
	// uncertainty must still fence the service even when no pair was returned.
	if errors.Is(task.err, ErrRecovery) {
		err = errors.Join(err, task.err)
	}
	return err
}
