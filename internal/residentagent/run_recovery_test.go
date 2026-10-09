package residentagent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productidentity"
)

func TestRecoveryStopIsCheckedAfterAgentOwnership(t *testing.T) {
	lease := &runLease{}
	owned := false
	failure := errors.New("superseded")
	err := run(context.Background(), t.TempDir(), runDependencies{
		acquire: func() (instanceLease, error) { owned = true; return lease, nil },
		checkStart: func() error {
			if !owned {
				t.Fatal("checked before ownership")
			}
			return failure
		},
		bootstrap: func(string, time.Time) (productidentity.Paths, *productidentity.Identity, bool, error) {
			t.Fatal("bootstrapped stale request")
			return productidentity.Paths{}, nil, false, nil
		},
		startMonitor: func() (agentlifecycle.Monitor, error) { return nil, nil },
		listen:       func(string, localapi.Operations) (apiServer, error) { return nil, nil }, now: time.Now,
	})
	if !errors.Is(err, failure) || !lease.closed {
		t.Fatal(err, lease.closed)
	}
}
