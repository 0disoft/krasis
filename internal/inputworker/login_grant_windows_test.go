//go:build windows

package inputworker

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/sys/windows"
)

func TestLoginGrantManagementRejectsUnprivilegedCaller(t *testing.T) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	if token.IsElevated() {
		t.Skip("do not mutate the attended machine's boot grant from an elevated test")
	}
	if ConfigureLoginGrant(context.Background(), "B") == nil {
		t.Fatal("ordinary caller configured boot")
	}
	if RemoveLoginGrant(context.Background()) == nil {
		t.Fatal("ordinary caller removed protected grant")
	}
}

func TestLoginConfigurationClosePreservesEffectUncertainty(t *testing.T) {
	cause := errors.New("native close failed")
	if err := closeLoginConfiguration(nil, cause, true); !errors.Is(err, ErrLoginConfigurationUnconfirmed) || !errors.Is(err, cause) {
		t.Fatal("post-change close lost uncertainty")
	}
	if err := closeLoginConfiguration(nil, cause, false); errors.Is(err, ErrLoginConfigurationUnconfirmed) || !errors.Is(err, cause) {
		t.Fatal("pre-change failure marked committed")
	}
	if err := closeLoginConfiguration(cause, nil, true); err != cause {
		t.Fatal("successful close changed operation error")
	}
}
