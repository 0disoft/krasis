//go:build windows

package inputworker

import "testing"

func TestSourceWriterCannotPreemptServiceReadinessWithEvents(t *testing.T) {
	w := &sourceWriter{}
	for _, kind := range []byte{sourceBoundary, sourceAction, sourceReturn} {
		if err := w.send(sourcePacket{kind: kind}, nil); err != nil || w.sequence != 0 {
			t.Fatal("event reached an unstarted stream", err)
		}
	}
}
