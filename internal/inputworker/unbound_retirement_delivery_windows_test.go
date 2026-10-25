//go:build windows

package inputworker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestNativeUnboundRetirementReceiptSurvivesWorkerCloseBeforeServiceRead(t *testing.T) {
	service, worker := nativePair(t)
	if err := writeFrame(worker, unboundRetiredFrame[:]); err != nil {
		t.Fatal("worker could not send retirement receipt", err)
	}
	if err := worker.Close(); err != nil {
		t.Fatal("worker close failed", err)
	}
	if err := readInputServiceFrame(service, unboundRetiredFrame); err != nil {
		t.Fatal("completed retirement receipt lost when worker closed before service read", err)
	}
}

func TestNativeCompletedPreparedStopRetiresAfterStartupCancellation(t *testing.T) {
	service, worker := nativePair(t)
	startup, cancel := context.WithCancel(context.Background())
	defer cancel()
	closedByStartup := make(chan struct{})
	stopControl := sync.OnceValue(context.AfterFunc(startup, func() {
		_ = worker.Close()
		close(closedByStartup)
	}))
	defer stopControl()
	done := make(chan error, 1)
	go func() {
		if err := writeFrame(worker, workerAuthenticatedFrame[:]); err != nil {
			done <- err
			return
		}
		packet, err := readPreparedBootstrap(startup, stopControl, func(ctx context.Context, data []byte) (int, error) {
			n, err := worker.readContext(ctx, data)
			cancel() // Deterministically deschedule after the kernel's completed read.
			return n, err
		})
		if err != nil || !zeroBytes(packet[:]) {
			done <- errors.Join(ErrProtocol, err)
			return
		}
		done <- retireUnboundNative(startup, worker, stopControl, func() error { return nil })
	}()
	if err := retireInputPreparation(service, workerAuthenticatedFrame, unboundRetiredFrame); err != nil {
		t.Fatal("completed stop lost its exact native retirement receipt", err)
	}
	if err := <-done; err != nil || !errors.Is(startup.Err(), context.Canceled) {
		t.Fatal("retirement failed or fixture did not cancel startup", err)
	}
	select {
	case <-closedByStartup:
		t.Fatal("startup callback closed the response pipe")
	default:
	}
}

func TestNativeCancelledPreparedReadRetainsResponsePipe(t *testing.T) {
	service, worker := nativePair(t)
	startup, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	stopControl := sync.OnceValue(context.AfterFunc(startup, func() { _ = worker.Close() }))
	defer stopControl()
	done := make(chan error, 1)
	go func() {
		_, err := readPreparedBootstrap(startup, stopControl, func(ctx context.Context, data []byte) (int, error) {
			cancel()
			return worker.readContext(ctx, data)
		})
		done <- err
	}()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("uncompleted bootstrap escaped startup cancellation", err)
	}
	if err := worker.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal("startup cancellation closed the response transport", err)
	}
	if err := writeFrame(worker, unboundRetiredFrame[:]); err != nil {
		t.Fatal("startup cancellation destroyed the duplex transport", err)
	}
	if err := readInputServiceFrame(service, unboundRetiredFrame); err != nil {
		t.Fatal(err)
	}
}
