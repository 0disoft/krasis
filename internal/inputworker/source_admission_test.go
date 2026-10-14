package inputworker

import (
	"errors"
	"io"
	"testing"

	"github.com/0disoft/krasis/internal/peer"
)

func TestSourceAdmissionNegativeQueryNeverBecomesPermission(t *testing.T) {
	p := sourcePacket{kind: sourceReply, op: sourceAdmit, status: 1, reply: 1}
	decision, err := sourceAdmissionResult(p, ErrAuthority)
	if decision != 0 || !errors.Is(err, peer.ErrBoundaryInspectionUnavailable) || !errors.Is(err, ErrAuthority) {
		t.Fatal("matching negative query granted admission or lost its failure", decision, err)
	}
	for _, failure := range []error{ErrProtocol, ErrRecovery, io.EOF, errors.Join(ErrAuthority, io.ErrClosedPipe)} {
		if decision, err := sourceAdmissionResult(p, failure); decision != 0 || errors.Is(err, peer.ErrBoundaryInspectionUnavailable) || !errors.Is(err, failure) {
			t.Fatal("transport/protocol/recovery failure became temporary query rejection", decision, err)
		}
	}
	for _, bad := range []sourcePacket{{}, {kind: sourceReply, op: sourceActivate, reply: 1, status: 1}, {kind: sourceReply, op: sourceAdmit, status: 1}} {
		if _, err := sourceAdmissionResult(bad, ErrAuthority); errors.Is(err, peer.ErrBoundaryInspectionUnavailable) {
			t.Fatal("unconfirmed query reply became temporary rejection")
		}
	}
}

func TestSourceAdmissionAllowedAndBlockedRemainDistinct(t *testing.T) {
	for _, decision := range []peer.BoundaryAdmissionDecision{peer.BoundaryAdmissionAllowed, peer.BoundaryAdmissionBlockedPressedInput, peer.BoundaryAdmissionBlockedLocalMenu} {
		actual, err := sourceAdmissionResult(sourcePacket{decision: decision}, nil)
		if err != nil || actual != decision {
			t.Fatal("known boundary decision changed", actual, err)
		}
	}
	if _, err := sourceAdmissionResult(sourcePacket{}, nil); !errors.Is(err, ErrProtocol) {
		t.Fatal("empty successful reply became admission")
	}
}
