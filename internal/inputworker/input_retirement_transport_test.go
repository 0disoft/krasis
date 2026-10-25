package inputworker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/logindiag"
)

func TestInputRetirementTransportClassSurvivesRuntimeReceipt(t *testing.T) {
	for _, stage := range []string{"authentication", "stop", "receipt"} {
		for _, tc := range []struct {
			class string
			cause error
		}{
			{"timeout", os.ErrDeadlineExceeded}, {"timeout", context.DeadlineExceeded},
			{"cancelled", context.Canceled}, {"truncated", io.ErrUnexpectedEOF},
			{"eof", io.EOF}, {"closed", net.ErrClosed}, {"closed", os.ErrClosed},
		} {
			t.Run(stage+"/"+tc.class, func(t *testing.T) {
				err := inputServiceTransportFailure("input-retire-"+stage, errors.Join(ErrRecovery, tc.cause, errors.New("SECRET/path")))
				if !errors.Is(err, ErrRecovery) || !errors.Is(err, tc.cause) {
					t.Fatal("classification changed recovery or I/O cause", err)
				}
				r := newLoginRuntimeRecorder(func(logindiag.Runtime) error { return nil })
				r.record("service", "FAILED", serviceEndpointFailure(err, "input"))
				if !r.value.Valid() || r.value.LastFailure == nil || r.value.LastFailure.Operation != "input-retire-"+stage+"-"+tc.class {
					t.Fatal("runtime lost bounded transport class", r.value.LastFailure)
				}
				data, err := json.Marshal(r.value)
				if err != nil || strings.Contains(string(data), "SECRET") {
					t.Fatal("private error text entered receipt", err)
				}
			})
		}
	}
	// Do not replace an already diagnosed native/worker failure with a class.
	specific := inputServiceAt("worker-report-timeout", errors.Join(ErrRecovery, io.EOF))
	if inputServiceTransportFailure("input-retire-receipt", specific) != specific {
		t.Fatal("specific operation was overwritten")
	}
}
