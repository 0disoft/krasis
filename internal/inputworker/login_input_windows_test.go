//go:build windows

package inputworker

import (
	"context"
	"testing"

	"github.com/0disoft/krasis/internal/application"
	"golang.org/x/sys/windows"
)

func TestLoginInputCannotBeEnabledByOrdinaryProcessCallback(t *testing.T) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	if user.User.Sid.IsWellKnown(windows.WinLocalSystemSid) {
		t.Skip("requires an ordinary user process")
	}
	called := false
	_, err = RunLoginInput(context.Background(), func() error { return nil }, func(context.Context, application.InputPort) error { called = true; return nil })
	if err == nil || called {
		t.Fatalf("ordinary process entered login input: err=%v called=%v", err, called)
	}
}
