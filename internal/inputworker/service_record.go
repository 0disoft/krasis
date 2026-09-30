package inputworker

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
)

type serviceRecordPhase uint8

const (
	serviceStarting serviceRecordPhase = iota + 1
	serviceReady
	serviceClean
	serviceFailed
)

const serviceRecordSize = 80

// An aggregate service receipt. It is not a lease authorization and contains
// neither input events nor identity data. Trust also requires protected storage,
// an approved service image and matching kernel process/SCM metadata.
type serviceRecord struct {
	Phase  serviceRecordPhase
	PID    uint32
	Run    Binding
	Digest [sha256.Size]byte
}

func encodeServiceRecord(record serviceRecord) ([serviceRecordSize]byte, error) {
	var data [serviceRecordSize]byte
	if record.Phase < serviceStarting || record.Phase > serviceFailed || record.PID == 0 || record.Run == (Binding{}) || record.Digest == ([sha256.Size]byte{}) {
		return data, ErrAuthority
	}
	copy(data[:4], "KRS1")
	data[4], data[5] = 1, byte(record.Phase)
	binary.BigEndian.PutUint32(data[8:12], record.PID)
	copy(data[16:32], record.Run[:])
	copy(data[32:64], record.Digest[:])
	return data, nil
}

func decodeServiceRecord(data []byte) (serviceRecord, error) {
	if len(data) != serviceRecordSize || string(data[:4]) != "KRS1" || data[4] != 1 || !zeroBytes(data[6:8]) || !zeroBytes(data[12:16]) || !zeroBytes(data[64:80]) {
		return serviceRecord{}, ErrAuthority
	}
	record := serviceRecord{Phase: serviceRecordPhase(data[5]), PID: binary.BigEndian.Uint32(data[8:12])}
	copy(record.Run[:], data[16:32])
	copy(record.Digest[:], data[32:64])
	if _, err := encodeServiceRecord(record); err != nil {
		return serviceRecord{}, err
	}
	return record, nil
}

// run must not accept input until ready succeeds. A failed readiness write is
// therefore a no-input startup failure. Once ready, only successful worker
// recovery can produce a clean receipt. Persist runs outside the cancelled ctx.
func runRecordedService(ctx context.Context, record serviceRecord, persist func(serviceRecord) error, run func(context.Context, func() error) error) error {
	if ctx == nil || persist == nil || run == nil {
		return ErrAuthority
	}
	record.Phase = serviceStarting
	if _, err := encodeServiceRecord(record); err != nil || persist(record) != nil {
		return ErrAuthority
	}
	ready := false
	err := run(ctx, func() error {
		if ready {
			return nil
		}
		record.Phase = serviceReady
		if persist(record) != nil {
			return ErrAuthority
		}
		ready = true
		return nil
	})
	record.Phase = serviceClean
	if err != nil && ready {
		record.Phase = serviceFailed
	}
	if persist(record) != nil {
		return ErrRecovery
	}
	return err
}
