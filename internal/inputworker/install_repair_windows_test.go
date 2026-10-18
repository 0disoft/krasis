//go:build windows

package inputworker

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"golang.org/x/sys/windows"
)

func TestStoppedServiceRepairRequiresRetiredFailedOwnerAndNeverRetries(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		phase                        serviceRecordPhase
		exited                       bool
		retireErr, startErr, stopErr error
		want                         []string
		operation                    string
	}{
		{"clean", serviceClean, true, nil, nil, nil, nil, ""},
		{"starting", serviceStarting, true, nil, nil, nil, nil, ""},
		{"ready", serviceReady, true, nil, nil, nil, nil, ""},
		{"live or reused PID", serviceFailed, false, nil, nil, nil, []string{"retired"}, "service_repair_retired"},
		{"unknown PID", serviceFailed, false, windows.ERROR_ACCESS_DENIED, nil, nil, []string{"retired"}, "service_repair_retired"},
		{"start failed", serviceFailed, true, nil, windows.ERROR_SERVICE_DISABLED, nil, []string{"retired", "start"}, "service_repair_start"},
		{"stop failed", serviceFailed, true, nil, nil, windows.ERROR_TIMEOUT, []string{"retired", "start", "stop"}, "service_repair_stop"},
		{"clean recovery", serviceFailed, true, nil, nil, nil, []string{"retired", "start", "stop"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			record := serviceRecord{Phase: tc.phase, PID: 42, Run: Binding{1}, Digest: [32]byte{2}}
			err := repairStoppedService(context.Background(), record, record.Digest,
				func(pid uint32) (bool, error) {
					if pid != record.PID {
						t.Fatal("wrong owner")
					}
					calls = append(calls, "retired")
					return tc.exited, tc.retireErr
				},
				func(context.Context) error { calls = append(calls, "start"); return tc.startErr },
				func(context.Context) error { calls = append(calls, "stop"); return tc.stopErr })
			if !reflect.DeepEqual(calls, tc.want) {
				t.Fatalf("calls %v, want %v", calls, tc.want)
			}
			if (err == nil) != (tc.name == "clean recovery") {
				t.Fatalf("unexpected result: %v", err)
			}
			if tc.operation != "" {
				op, code, known := installErrorFields(err)
				if op != tc.operation {
					t.Fatal("lost repair stage", op)
				}
				for _, native := range []error{tc.retireErr, tc.startErr, tc.stopErr} {
					if native != nil && (!errors.Is(err, native) || !known || code == 0) {
						t.Fatal("lost native failure")
					}
				}
			}
		})
	}
}

func TestStoppedServiceRepairRefusesCancelledOrMismatchedEvidence(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		ctx    context.Context
		record serviceRecord
	}{
		{ctx, serviceRecord{Phase: serviceFailed, PID: 42, Run: Binding{1}, Digest: [32]byte{2}}},
		{context.Background(), serviceRecord{Phase: serviceFailed, PID: 42, Run: Binding{1}, Digest: [32]byte{3}}},
		{context.Background(), serviceRecord{Phase: serviceFailed, PID: 0, Run: Binding{1}, Digest: [32]byte{2}}},
	} {
		if repairStoppedService(tc.ctx, tc.record, [32]byte{2}, func(uint32) (bool, error) { t.Fatal("unexpected process query"); return false, nil }, nil, nil) == nil {
			t.Fatal("accepted invalid recovery evidence")
		}
	}
	if retired, err := retiredServiceProcess(windows.GetCurrentProcessId()); err != nil || retired {
		t.Fatal("live process accepted as retired", err)
	}
}
