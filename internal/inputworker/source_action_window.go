package inputworker

import (
	"context"
	"sync"
)

const sourceActionReceiptEvery = 8
const sourceActionWindowLimit = 2 * sourceActionReceiptEvery
const sourceReceiptQueueCapacity = 1 + sourceActionWindowLimit/sourceActionReceiptEvery

// Reserve at most 1KiB of the 4KiB pipe for unconsumed input. Credit waits
// never own the wire gate. Receipts certify consumption, not input authority,
// remote application, liveness or native retirement.
type sourceActionWindow struct {
	credit      chan struct{}
	mu          sync.Mutex
	sequences   [sourceActionWindowLimit]uint64
	head, count int
}

func newSourceActionWindow() *sourceActionWindow {
	w := &sourceActionWindow{credit: make(chan struct{}, sourceActionWindowLimit)}
	for range sourceActionWindowLimit {
		w.credit <- struct{}{}
	}
	return w
}

func (w *sourceActionWindow) acquire(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.credit:
	}
	if err := ctx.Err(); err != nil {
		w.credit <- struct{}{}
		return err
	}
	return nil
}

// The serial writer publishes before submission: a fast reader can return a
// receipt before Write wakes up. On an unsubmitted failure the writer rolls
// back only its own last entry; attempted writes remain terminal.
func (w *sourceActionWindow) append(sequence uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.sequences[(w.head+w.count)%sourceActionWindowLimit] = sequence
	w.count++
}

func (w *sourceActionWindow) rollback(sequence uint64) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.count > 0 && w.sequences[(w.head+w.count-1)%sourceActionWindowLimit] == sequence {
		w.count--
		return true
	}
	return false // The reader already consumed and returned this credit.
}

func (w *sourceActionWindow) acknowledge(sequence uint64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.count < sourceActionReceiptEvery || sequence == 0 || w.sequences[(w.head+sourceActionReceiptEvery-1)%sourceActionWindowLimit] != sequence {
		return ErrProtocol
	}
	w.head = (w.head + sourceActionReceiptEvery) % sourceActionWindowLimit
	w.count -= sourceActionReceiptEvery
	for range sourceActionReceiptEvery {
		w.credit <- struct{}{}
	}
	return nil
}
