package inputworker

import (
	"context"
	"testing"
)

func testServiceRecord() serviceRecord {
	return serviceRecord{PID: 123, Run: Binding{1}, Digest: installTestImages("service").Service, Phase: serviceStarting}
}

func TestServiceReceiptReadyPrecedesInputAndCleanFollowsRecovery(t *testing.T) {
	var phases []serviceRecordPhase
	persist := func(record serviceRecord) error { phases = append(phases, record.Phase); return nil }
	err := runRecordedService(context.Background(), testServiceRecord(), persist, func(ctx context.Context, ready func() error) error {
		if ready() != nil || ready() != nil || len(phases) != 2 || phases[1] != serviceReady {
			t.Fatal("ready not persisted exactly once before input")
		}
		return nil
	})
	if err != nil || len(phases) != 3 || phases[2] != serviceClean {
		t.Fatal("clean service receipt missing")
	}
}

func TestServiceReceiptFailureCannotClaimCleanAfterInput(t *testing.T) {
	var final serviceRecordPhase
	err := runRecordedService(context.Background(), testServiceRecord(), func(record serviceRecord) error { final = record.Phase; return nil }, func(ctx context.Context, ready func() error) error {
		if ready() != nil {
			t.Fatal("readiness failed")
		}
		return ErrRecovery
	})
	if err != ErrRecovery || final != serviceFailed {
		t.Fatal("worker recovery failure produced a clean receipt")
	}
}

func TestServiceReceiptReadinessFailureProvesNoInput(t *testing.T) {
	var phases []serviceRecordPhase
	err := runRecordedService(context.Background(), testServiceRecord(), func(record serviceRecord) error {
		phases = append(phases, record.Phase)
		if record.Phase == serviceReady {
			return ErrAuthority
		}
		return nil
	}, func(ctx context.Context, ready func() error) error {
		if ready() == nil {
			t.Fatal("failed persistence enabled input")
		}
		return ErrAuthority
	})
	if err != ErrAuthority || phases[len(phases)-1] != serviceClean {
		t.Fatal("no-input startup failure lost its recovery receipt")
	}
}

func TestServiceReceiptCancelledStopStillPersistsAndWriteFailureStopsSCM(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := runRecordedService(ctx, testServiceRecord(), func(record serviceRecord) error {
		if record.Phase == serviceClean {
			if ctx.Err() == nil {
				t.Fatal("fixture did not cancel service")
			}
			return ErrAuthority
		}
		return nil
	}, func(ctx context.Context, ready func() error) error { cancel(); return nil })
	if err != ErrRecovery {
		t.Fatal("missing durable stop receipt was hidden")
	}
}

func TestServiceReceiptWireRejectsInvalidSizePaddingAndIdentity(t *testing.T) {
	record := testServiceRecord()
	data, err := encodeServiceRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := decodeServiceRecord(data[:]); err != nil || decoded != record {
		t.Fatal("service receipt roundtrip failed")
	}
	for _, offset := range []int{0, 4, 6, 12, 64, 79} {
		bad := data
		bad[offset] ^= 0xff
		if _, err := decodeServiceRecord(bad[:]); err != ErrAuthority {
			t.Fatal("malformed service receipt accepted")
		}
	}
	for _, bad := range []serviceRecord{{}, {PID: 1, Phase: serviceReady, Run: Binding{1}}, {PID: 1, Phase: 99, Run: Binding{1}, Digest: record.Digest}} {
		if _, err := encodeServiceRecord(bad); err != ErrAuthority {
			t.Fatal("empty service identity accepted")
		}
	}
}
