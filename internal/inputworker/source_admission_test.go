package inputworker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"syscall"
	"testing"

	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/wininput"
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

func TestSourceNegativeQueryRequiresCleanGUIInspectionFailure(t *testing.T) {
	query := fmt.Errorf("foreground query: %w", errors.Join(peer.ErrBoundaryInspectionUnavailable, syscall.Errno(5)))
	if !sourceRetryableInspection(query) {
		t.Fatal("real GUI query failure not retryable")
	}
	for _, other := range []error{ErrAuthority, ErrProtocol, ErrRecovery, context.Canceled, context.DeadlineExceeded, inputdesktop.ErrCleanup, inputdesktop.ErrInactive, inputdesktop.ErrCallbackPanic, inputdesktop.ErrCallbackExit, wininput.ErrSourceCleanupIncomplete} {
		if sourceRetryableInspection(other) || sourceRetryableInspection(errors.Join(query, other)) {
			t.Fatal("authority/retirement failure became GUI rejection", other)
		}
	}
	result := sourceCaptureFinal(&sourceInspectionRejected{cause: query}, inputdesktop.ErrInactive, context.Canceled, false)
	if result == nil || errors.Is(result, ErrRecovery) || !errors.Is(result, query) {
		t.Fatal("clean inspection failure lost or fenced", result)
	}
	var wire bytes.Buffer
	if err := writeReport(&wire, Report{}, result); err != nil {
		t.Fatal(err)
	}
	if wire.Bytes()[5] != 1 {
		t.Fatal("rejected command became success")
	}
	_, received := readReport(&wire)
	if received == nil || serviceRecoveryFailure(received, nil) != nil {
		t.Fatal("failure not preserved independently of native retirement", received)
	}
	for _, native := range []error{inputdesktop.ErrCleanup, wininput.ErrSourceCleanupIncomplete} {
		unsafe := &sourceInspectionRejected{cause: errors.Join(query, native)}
		if result := sourceCaptureFinal(unsafe, inputdesktop.ErrInactive, context.Canceled, false); !errors.Is(result, ErrRecovery) || sourceRetirementStatus(result) != 1 {
			t.Fatal("inspection wrapper hid incomplete cleanup", result)
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
