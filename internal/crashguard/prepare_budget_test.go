package crashguard

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestPortablePreparationSharesCallerBudgetAcrossAdmissionAndAck(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		input, guardian := closeTestProcess(t), closeTestProcess(t)
		processes := &PortableProcesses{input: input, guardian: guardian}
		reader, writer := io.Pipe()
		defer reader.Close()
		defer writer.Close()
		guardian.client, _ = NewClient(&bytes.Buffer{}, reader)
		processes.gate.Lock()
		go func() { time.Sleep(30 * time.Millisecond); processes.gate.Unlock() }()
		ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
		defer cancel()
		started := time.Now()
		prepared, err := processes.PreparePressContext(ctx, control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30})
		if prepared || !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != 75*time.Millisecond {
			t.Fatal("preparation renewed the deadline after admission", prepared, err, time.Since(started))
		}
		if !guardian.failed.Load() || !processes.inputFenced.Load() {
			t.Fatal("expired admitted exchange did not fence its owners")
		}
		if err := writeAck(writer, operationPreparePress, statusOK); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if prepared, err := processes.PreparePressContext(context.Background(), control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}); prepared || err == nil {
			t.Fatal("late acknowledgement revived the retired guardian", prepared, err)
		}
		close(input.done)
		close(guardian.done)
		_ = processes.Close()
	})
}

func TestExpiredPreparationAdmissionDoesNotRetireAnotherOwner(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		input, guardian := closeTestProcess(t), closeTestProcess(t)
		processes := &PortableProcesses{input: input, guardian: guardian}
		processes.gate.Lock()
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		if prepared, err := processes.PreparePressContext(ctx, control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}); prepared || !errors.Is(err, context.DeadlineExceeded) || guardian.failed.Load() || processes.inputFenced.Load() {
			t.Fatal("unadmitted preparation changed the live protocol owner", prepared, err)
		}
		processes.gate.Unlock()
		var ack bytes.Buffer
		_ = writeAck(&ack, operationPreparePress, statusOK)
		guardian.client, _ = NewClient(&bytes.Buffer{}, &ack)
		if prepared, err := processes.PreparePressContext(context.Background(), control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}); !prepared || err != nil {
			t.Fatal("fresh preparation lost usable owner after an expired waiter", prepared, err)
		}
		close(input.done)
		close(guardian.done)
		_ = processes.Close()
	})
}
