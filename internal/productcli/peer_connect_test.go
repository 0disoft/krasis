package productcli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/0disoft/krasis/internal/localapi"
)

func TestCLIConnectWaitsForExactObservedConnectionAndNeverRetriesStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		id := strings.Repeat("b", 32)
		starts, polls := 0, 0
		client := scriptedAppOpenClient(func(_ context.Context, r localapi.AppOpenRequest) (localapi.AppOpenResult, error) {
			if r.Action == "CONNECT" {
				starts++
				return localapi.AppOpenResult{Code: "queued", ID: id}, nil
			}
			if r.Action != "CONNECT_STATUS" || r.ID != id {
				t.Fatal("wrong request")
			}
			polls++
			code := "accepted"
			if polls == 3 {
				code = "connected"
			}
			return localapi.AppOpenResult{Code: code, ID: id}, nil
		})
		var out, errs bytes.Buffer
		if exit := run(context.Background(), []string{"app", "connect", "B", "--json"}, &out, &errs, testDependencies(client)); exit != ExitOK || !strings.Contains(out.String(), `"code":"connected"`) || starts != 1 || polls != 3 {
			t.Fatal(exit, out.String(), errs.String(), starts, polls)
		}
	})
}

func TestCLIConnectUnknownDeliveryAndStatusNeverSubmitAgain(t *testing.T) {
	for _, query := range []bool{false, true} {
		starts := 0
		client := scriptedAppOpenClient(func(_ context.Context, r localapi.AppOpenRequest) (localapi.AppOpenResult, error) {
			if r.Action == "CONNECT" {
				starts++
				return localapi.AppOpenResult{}, errors.New("lost response")
			}
			return localapi.AppOpenResult{Code: "connected", ID: strings.Repeat("c", 32)}, nil
		})
		args := []string{"app", "connect", "B", "--json"}
		if query {
			args = []string{"app", "connect-status", "B", strings.Repeat("b", 32), "--json"}
		}
		var out, errs bytes.Buffer
		if exit := run(context.Background(), args, &out, &errs, testDependencies(client)); exit == ExitOK || !strings.Contains(out.String(), "outcome_unknown") {
			t.Fatal(exit, out.String())
		}
		if (!query && starts != 1) || (query && starts != 0) {
			t.Fatal("resubmitted", starts)
		}
	}
}
