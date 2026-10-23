package inputworker

import (
	"bytes"
	"context"
	"testing"

	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/wininput"
)

func TestSourceFinalPacketKeepsEmergencyAfterStopOrCommandFailure(t *testing.T) {
	for _, stopping := range []*sourcePacket{nil, {sequence: 19}} {
		for _, loop := range []error{context.Canceled, ErrAuthority} {
			result := sourceCaptureFinal(loop, wininput.ErrEmergencyRelease, context.Canceled, false)
			response := sourceFinalPacket(stopping, result, true)
			if response.kind != sourceRetired || response.op != 1 || response.status != 0 || response.reply != 0 {
				t.Error("clean emergency became a restartable stop reply", response)
			}
			// The writer supplies the authenticated sequence/binding. Use the
			// existing encoder/parser to ensure no packet extension is required.
			response.sequence, response.binding = 20, Binding{1}
			encoded, err := encodeSourcePacket(response)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := readSourcePacket(bytes.NewReader(encoded[:]))
			if err != nil || decoded != response {
				t.Fatal("emergency packet did not survive the existing protocol", err)
			}
		}
	}
}

func TestSourceFinalPacketRetainsCleanupFailureAndOrdinaryStopAck(t *testing.T) {
	for _, stopping := range []*sourcePacket{nil, {sequence: 19}} {
		for _, emergency := range []bool{false, true} {
			result := sourceCaptureFinal(context.Canceled, inputdesktop.ErrCleanup, context.Canceled, false)
			response := sourceFinalPacket(stopping, result, emergency)
			if response.status != 1 || stopping == nil && response.op != 0 ||
				stopping != nil && (response.kind != sourceReply || response.op != sourceStop || response.reply != stopping.sequence) {
				t.Fatal("uncertain cleanup became clean emergency proof", response)
			}
		}
	}
	response := sourceFinalPacket(&sourcePacket{sequence: 19}, nil, false)
	if response.kind != sourceReply || response.op != sourceStop || response.reply != 19 || response.status != 0 {
		t.Fatal("ordinary confirmed shutdown lost its exact acknowledgment", response)
	}
}
