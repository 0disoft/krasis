package inputworker

import (
	"context"
	"testing"
)

func TestSourceDispatchPrioritizesPendingActivationWithoutDiscardingBoundaryOrder(t *testing.T) {
	commands, boundary := make(chan sourcePacket, 1), make(chan sourcePacket, 64)
	returns := make(chan uint64, 1)
	for n := 0; n < 64; n++ {
		boundary <- sourcePacket{kind: sourceBoundary, x: int32(n)}
	}
	commands <- sourcePacket{kind: sourceCommand, op: sourceActivate, sequence: 42}
	work, err := nextSourceWork(context.Background(), commands, boundary, returns)
	if err != nil || work.packet.op != sourceActivate || work.packet.sequence != 42 || len(boundary) != 64 {
		t.Fatal("pointer backlog delayed activation or was discarded", work, err)
	}
	for n := 0; n < 64; n++ {
		work, err = nextSourceWork(context.Background(), commands, boundary, returns)
		if err != nil || work.packet.kind != sourceBoundary || work.packet.x != int32(n) {
			t.Fatal("pointer queue reordered", n, work, err)
		}
	}
	returns <- 17
	work, err = nextSourceWork(context.Background(), commands, boundary, returns)
	if err != nil || work.packet.kind != sourceReturn || work.returnGeneration != 17 {
		t.Fatal("local return lost", work, err)
	}
}

func TestSourceDispatchCancellationCannotExecuteQueuedCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	commands := make(chan sourcePacket, 1)
	commands <- sourcePacket{kind: sourceCommand, op: sourceActivate, sequence: 1}
	if _, err := nextSourceWork(ctx, commands, nil, nil); err != context.Canceled || len(commands) != 1 {
		t.Fatal("cancelled worker consumed command", err)
	}
}
