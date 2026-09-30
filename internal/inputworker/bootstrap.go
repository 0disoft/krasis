package inputworker

import (
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/0disoft/krasis/internal/application"
)

const (
	leaseSize        = 432
	bootstrapSize    = 16 + leaseSize
	reportSize       = 32
	startupTimeout   = 5 * time.Second
	maxLeaseDuration = time.Hour
)

var readyFrame = [8]byte{'K', 'R', 'D', '1', 1, 0, 0, 0}

func encodeLease(lease Lease, now time.Time) ([leaseSize]byte, error) {
	var data [leaseSize]byte
	if !lease.ExpiresAt.After(now) || lease.ExpiresAt.After(now.Add(maxLeaseDuration)) || lease.Binding == (Binding{}) {
		return data, ErrProtocol
	}
	copy(data[:4], "KRL1")
	data[4] = 1
	binary.BigEndian.PutUint64(data[8:16], uint64(lease.ExpiresAt.UnixMilli()))
	copy(data[16:32], lease.Binding[:])
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
	if string(data[:4]) != "KRL1" || data[4] != 1 || !zeroBytes(data[5:8]) || !zeroBytes(data[38:40]) || !zeroBytes(data[424:]) {
		return Lease{}, ErrProtocol
	}
	lease := Lease{ExpiresAt: time.UnixMilli(int64(binary.BigEndian.Uint64(data[8:16])))}
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
	data[4] = 1
	if result != nil {
		data[5] = 1
	}
	if errors.Is(result, ErrRecovery) || report.Recovery.Remaining != 0 {
		data[5] = 2
	}
	binary.BigEndian.PutUint64(data[8:16], report.Completed)
	for i, value := range []int{report.Recovery.Attempted, report.Recovery.Released, report.Recovery.Remaining} {
		if value < 0 || value > maxPressed {
			return ErrProtocol
		}
		binary.BigEndian.PutUint32(data[16+i*4:20+i*4], uint32(value))
	}
	return writeFrame(writer, data[:])
}

func readReport(reader io.Reader) (Report, error) {
	var data [reportSize]byte
	if _, err := io.ReadFull(reader, data[:]); err != nil || string(data[:4]) != "KRR1" || data[4] != 1 || data[5] > 2 || !zeroBytes(data[6:8]) || !zeroBytes(data[28:]) {
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
	if data[5] == 2 {
		return report, ErrRecovery
	}
	if data[5] == 1 {
		return report, ErrAuthority
	}
	return report, nil
}
