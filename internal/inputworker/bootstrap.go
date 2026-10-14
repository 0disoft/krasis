package inputworker

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/inputdesktop"
)

const (
	leaseSize            = 432
	bootstrapLeaseOffset = 24 + sha256.Size
	bootstrapSize        = bootstrapLeaseOffset + leaseSize
	reportSize           = 32
	startupTimeout       = 5 * time.Second
	maxLeaseDuration     = time.Hour
)

var readyFrame = [8]byte{'K', 'R', 'D', '1', 1, 0, 0, 0}

// Distinct from readiness: this only acknowledges independent service auth.
var workerAuthenticatedFrame = [8]byte{'K', 'R', 'A', '1', 1, 0, 0, 0}

func readWorkerAuthenticated(reader io.Reader) error {
	var data [8]byte
	if _, err := io.ReadFull(reader, data[:]); err != nil || data != workerAuthenticatedFrame {
		return ErrProtocol
	}
	return nil
}

// Native handle transfer is unreachable until the exact peer's authentication
// acknowledgement and fresh authority checks have both completed.
func bootstrapAfterAuthentication(ctx context.Context, reader io.Reader, check func() error, transfer func() ([bootstrapSize]byte, error)) ([bootstrapSize]byte, error) {
	if ctx == nil || ctx.Err() != nil || check == nil || transfer == nil {
		return [bootstrapSize]byte{}, ErrAuthority
	}
	if readWorkerAuthenticated(reader) != nil || ctx.Err() != nil || check() != nil || ctx.Err() != nil {
		return [bootstrapSize]byte{}, ErrAuthority
	}
	return transfer()
}

func encodeWorkerBootstrap(pipe, image uint64, agent [sha256.Size]byte, lease [leaseSize]byte) ([bootstrapSize]byte, error) {
	var data [bootstrapSize]byte
	if !validBootstrapHandles(pipe, image) || agent == ([sha256.Size]byte{}) {
		return data, ErrProtocol
	}
	copy(data[:4], "KRB2")
	data[4] = 2
	binary.BigEndian.PutUint64(data[8:16], pipe)
	binary.BigEndian.PutUint64(data[16:24], image)
	copy(data[24:bootstrapLeaseOffset], agent[:])
	copy(data[bootstrapLeaseOffset:], lease[:])
	return data, nil
}

func decodeWorkerBootstrapHandles(data [bootstrapSize]byte, expectedAgent [sha256.Size]byte) (uint64, uint64, error) {
	pipe, image := binary.BigEndian.Uint64(data[8:16]), binary.BigEndian.Uint64(data[16:24])
	var agent [sha256.Size]byte
	copy(agent[:], data[24:bootstrapLeaseOffset])
	if string(data[:4]) != "KRB2" || data[4] != 2 || !zeroBytes(data[5:8]) || !validBootstrapHandles(pipe, image) || expectedAgent == ([sha256.Size]byte{}) || agent != expectedAgent {
		return 0, 0, ErrProtocol
	}
	return pipe, image, nil
}

func validBootstrapHandles(pipe, image uint64) bool {
	for _, handle := range []uint64{pipe, image} {
		if handle == 0 || uint64(uintptr(handle)) != handle || uintptr(handle) == ^uintptr(0) {
			return false
		}
	}
	return pipe != image
}

func encodeLease(lease Lease, now time.Time) ([leaseSize]byte, error) {
	var data [leaseSize]byte
	if !lease.ExpiresAt.After(now) || lease.ExpiresAt.After(now.Add(maxLeaseDuration)) || lease.Binding == (Binding{}) {
		return data, ErrProtocol
	}
	copy(data[:4], "KRL1")
	data[4] = 1
	binary.BigEndian.PutUint64(data[8:16], uint64(lease.ExpiresAt.UnixMilli()))
	copy(data[16:32], lease.Binding[:])
	if lease.Capture {
		if lease.Presentation || lease.CursorX != 0 || lease.CursorY != 0 {
			return data, ErrProtocol
		}
		copy(data[:4], "KRC1")
		data[38] = 2
	} else if lease.Presentation {
		if lease.ExpiresAt.After(now.Add(presentationWindow)) {
			return [leaseSize]byte{}, ErrProtocol
		}
		copy(data[:4], "KRS1")
		data[38] = 1
		binary.BigEndian.PutUint32(data[424:428], uint32(lease.CursorX))
		binary.BigEndian.PutUint32(data[428:432], uint32(lease.CursorY))
	} else if lease.CursorX != 0 || lease.CursorY != 0 {
		return [leaseSize]byte{}, ErrProtocol
	}
	for i, id := range []string{lease.Identity.SessionID, lease.Identity.LeaseID, lease.Identity.Epoch} {
		if len(id) == 0 || len(id) > 128 || !utf8.ValidString(id) || strings.ContainsRune(id, '\x00') {
			return [leaseSize]byte{}, ErrProtocol
		}
		binary.BigEndian.PutUint16(data[32+i*2:34+i*2], uint16(len(id)))
		copy(data[40+i*128:40+(i+1)*128], id)
	}
	return data, nil
}

func decodeLease(data [leaseSize]byte, now time.Time) (Lease, error) {
	presentation := string(data[:4]) == "KRS1"
	capture := string(data[:4]) == "KRC1"
	if (!presentation && !capture && string(data[:4]) != "KRL1") || data[4] != 1 || !zeroBytes(data[5:8]) || data[39] != 0 ||
		(presentation && data[38] != 1) || (capture && (data[38] != 2 || !zeroBytes(data[424:]))) || (!presentation && !capture && (!zeroBytes(data[38:40]) || !zeroBytes(data[424:]))) {
		return Lease{}, ErrProtocol
	}
	lease := Lease{ExpiresAt: time.UnixMilli(int64(binary.BigEndian.Uint64(data[8:16])))}
	lease.Presentation = data[38] == 1
	lease.Capture = capture
	if lease.Presentation {
		lease.CursorX = int32(binary.BigEndian.Uint32(data[424:428]))
		lease.CursorY = int32(binary.BigEndian.Uint32(data[428:432]))
	}
	copy(lease.Binding[:], data[16:32])
	ids := []*string{&lease.Identity.SessionID, &lease.Identity.LeaseID, &lease.Identity.Epoch}
	for i, id := range ids {
		length := int(binary.BigEndian.Uint16(data[32+i*2 : 34+i*2]))
		if length < 1 || length > 128 {
			return Lease{}, ErrProtocol
		}
		field := data[40+i*128 : 40+(i+1)*128]
		if !zeroBytes(field[length:]) {
			return Lease{}, ErrProtocol
		}
		*id = string(field[:length])
	}
	if _, err := encodeLease(lease, now); err != nil {
		return Lease{}, ErrProtocol
	}
	return lease, nil
}

func zeroBytes(data []byte) bool {
	for _, b := range data {
		if b != 0 {
			return false
		}
	}
	return true
}

func writeFrame(writer io.Writer, data []byte) error {
	n, err := writer.Write(data)
	if err != nil || n != len(data) {
		return ErrProtocol
	}
	return nil
}

func readReady(reader io.Reader) error {
	var data [8]byte
	if _, err := io.ReadFull(reader, data[:]); err != nil || data != readyFrame {
		return ErrProtocol
	}
	return nil
}

func writeReport(writer io.Writer, report Report, result error) error {
	var data [reportSize]byte
	copy(data[:4], "KRR1")
	data[4] = 2
	if result != nil {
		data[5] = 1
	}
	if errors.Is(result, ErrRecovery) || errors.Is(result, inputdesktop.ErrCleanup) || report.Recovery.Remaining != 0 {
		data[5] = 2
	}
	operation, native := reportFailureDetails(result)
	data[6] = operation
	binary.BigEndian.PutUint32(data[28:32], native)
	binary.BigEndian.PutUint64(data[8:16], report.Completed)
	for i, value := range []int{report.Recovery.Attempted, report.Recovery.Released, report.Recovery.Remaining} {
		if value < 0 || value > maxPressed {
			return ErrProtocol
		}
		binary.BigEndian.PutUint32(data[16+i*4:20+i*4], uint32(value))
	}
	return writeFrame(writer, data[:])
}

// Only a requested, pure cancellation can become a clean stop. An empty
// pressed journal does not prove that desktop restoration/handle cleanup worked.
func normalizeWorkerStop(requested bool, report Report, result error) error {
	if requested && report.Recovery.Remaining == 0 && errors.Is(result, context.Canceled) && workerCancellationOnly(result) {
		return nil
	}
	return result
}

func workerCancellationOnly(err error) bool {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range joined.Unwrap() {
			if !workerCancellationOnly(child) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return workerCancellationOnly(wrapped.Unwrap())
	}
	return err == context.Canceled
}

func readReport(reader io.Reader) (Report, error) {
	var data [reportSize]byte
	if _, err := io.ReadFull(reader, data[:]); err != nil {
		operation := "worker-report-read"
		if err == io.EOF {
			operation = "worker-report-eof"
		}
		if err == io.ErrUnexpectedEOF {
			operation = "worker-report-truncated"
		}
		if errors.Is(err, os.ErrDeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			operation = "worker-report-timeout"
		}
		return Report{}, &workerReportFailure{operation: operation, cause: errors.Join(ErrProtocol, err)}
	}
	if string(data[:4]) != "KRR1" || (data[4] != 1 && data[4] != 2) || data[5] > 2 || data[7] != 0 || !validReportOperation(data[6]) {
		return Report{}, ErrProtocol
	}
	if data[4] == 1 && (!zeroBytes(data[6:8]) || !zeroBytes(data[28:])) || data[5] == 0 && (data[6] != 0 || !zeroBytes(data[28:])) {
		return Report{}, ErrProtocol
	}
	values := [3]int{}
	for i := range values {
		value := binary.BigEndian.Uint32(data[16+i*4 : 20+i*4])
		if value > maxPressed {
			return Report{}, ErrProtocol
		}
		values[i] = int(value)
	}
	if values[1] > values[0] || (data[5] == 0 && values[2] != 0) || (values[2] != 0 && data[5] != 2) {
		return Report{}, ErrProtocol
	}
	report := Report{Completed: binary.BigEndian.Uint64(data[8:16]), Recovery: application.ReleaseReport{Attempted: values[0], Released: values[1], Remaining: values[2]}}
	return report, reportFailure(data[5], data[6], binary.BigEndian.Uint32(data[28:32]))
}
