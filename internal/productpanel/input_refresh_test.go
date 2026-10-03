package productpanel

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
)

func TestInputRefreshAutomaticConsentPolicy(t *testing.T) {
	for _, tc := range []struct {
		initial  string
		result   error
		verified bool
		want     string
		calls    int
		blocked  bool
	}{
		{"NOT_INSTALLED", nil, false, "NOT_INSTALLED", 0, false},
		{"UNKNOWN", nil, false, "UNKNOWN", 0, false},
		{"READY", nil, true, "READY", 0, false},
		{"UPDATE_REQUIRED", nil, true, "READY", 1, false},
		{"UPDATE_REQUIRED", nil, false, "FAILED", 1, false},
		{"UPDATE_REQUIRED", ErrInputRefreshCancelled, false, "CANCELLED", 1, false},
		{"UPDATE_REQUIRED", ErrInputRefreshFailed, false, "FAILED", 1, false},
		{"UPDATE_REQUIRED", ErrInputRefreshPending, false, "PENDING", 1, true},
	} {
		t.Run(tc.initial+tc.want, func(t *testing.T) {
			state, calls := tc.initial, 0
			b := &inputRefresh{inspect: func() string { return state }, run: func(context.Context) error {
				calls++
				if tc.verified {
					state = "READY"
				}
				return tc.result
			}}
			for range 3 {
				err := b.automatic(context.Background())
				if (err != nil) != tc.blocked {
					t.Fatalf("error %v", err)
				}
			}
			if b.status() != tc.want || calls != tc.calls {
				t.Fatalf("state=%s calls=%d", b.status(), calls)
			}
		})
	}
}

func TestInputRefreshRequiresExplicitRetryAndReverification(t *testing.T) {
	state, calls := "UPDATE_REQUIRED", 0
	b := &inputRefresh{inspect: func() string { return state }, run: func(context.Context) error {
		calls++
		if calls == 1 {
			return ErrInputRefreshCancelled
		}
		state = "READY"
		return nil
	}}
	if b.automatic(context.Background()) != nil || b.status() != "CANCELLED" {
		t.Fatal("cancel failed")
	}
	if b.automatic(context.Background()) != nil || calls != 1 {
		t.Fatal("automatic retry")
	}
	if b.explicit(context.Background()) != nil || b.status() != "READY" || calls != 2 {
		t.Fatal("explicit retry failed")
	}
	state = "UNKNOWN"
	if !errors.Is(b.explicit(context.Background()), ErrInputRefreshFailed) || calls != 2 {
		t.Fatal("unverified installation invoked runner")
	}
}

func TestInputRefreshShowsProgressWithoutBlockingStatus(t *testing.T) {
	started, done := make(chan struct{}), make(chan struct{})
	b := &inputRefresh{inspect: func() string { return "UPDATE_REQUIRED" }, run: func(context.Context) error { close(started); <-done; return ErrInputRefreshCancelled }}
	result := make(chan error, 1)
	go func() { result <- b.automatic(context.Background()) }()
	<-started
	if b.status() != "UPDATING" || !errors.Is(b.explicit(context.Background()), ErrStartupBusy) {
		t.Fatal("concurrent operation not fenced")
	}
	close(done)
	<-result
}

func TestStartupRefreshRunsOnlyAfterOwnershipAndBundleValidation(t *testing.T) {
	starts, prepares := 0, 0
	s := Startup{Lock: func() (io.Closer, error) { return closerStub{}, nil }, Open: func() (RuntimeClient, error) { return nil, localapi.ErrUnavailable },
		Available: func() (bool, error) { return true, nil }, Verify: func() error { return errors.New("tampered") }, BeforeStart: func(context.Context) error { prepares++; return nil }, Start: func() error { starts++; return nil }}
	if s.Ensure(context.Background()) == nil || starts != 0 || prepares != 0 {
		t.Fatal("unverified bundle invoked installer")
	}
}
