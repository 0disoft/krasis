package inputworker

import "context"

type sourceWork struct {
	packet           sourcePacket
	returnGeneration uint64
}

// Commands have already passed the serial reader's binding, sequence and
// authority checks. Prefer one pending command over unsent pointer telemetry;
// never reorder packets already written or any items within an event queue.
func nextSourceWork(ctx context.Context, commands <-chan sourcePacket, boundary <-chan sourcePacket, returns <-chan uint64) (sourceWork, error) {
	if err := ctx.Err(); err != nil {
		return sourceWork{}, err
	}
	select {
	case p := <-commands:
		return sourceWork{packet: p}, nil
	default:
	}
	select {
	case <-ctx.Done():
		return sourceWork{}, ctx.Err()
	case p := <-commands:
		return sourceWork{packet: p}, nil
	case p := <-boundary:
		return sourceWork{packet: p}, nil
	case generation := <-returns:
		return sourceWork{packet: sourcePacket{kind: sourceReturn}, returnGeneration: generation}, nil
	}
}
