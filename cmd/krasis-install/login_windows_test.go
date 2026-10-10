//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/inputworker"
)

func TestLoginConfigurationArgumentsAndHonestInactiveReceipt(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		called  bool
		handled bool
	}{
		{[]string{"--prepare-login", "B"}, true, true},
		{[]string{"--disable-login"}, true, true},
		{[]string{"--prepare-login"}, false, true},
		{[]string{"--prepare-login", " B"}, false, true},
		{[]string{"--prepare-login", "--path"}, false, true},
		{[]string{"--disable-login", "B"}, false, true},
		{[]string{"--install"}, false, false},
	} {
		var out bytes.Buffer
		called := false
		check := func(ctx context.Context) {
			called = true
			if _, ok := ctx.Deadline(); !ok || ctx.Err() != nil {
				t.Fatal("configuration is unbounded")
			}
		}
		code, handled := runLoginConfiguration(tc.args, &out, func(ctx context.Context, alias string) error {
			check(ctx)
			if alias != "B" {
				t.Fatal("alias changed")
			}
			return nil
		}, func(ctx context.Context) error { check(ctx); return nil })
		if handled != tc.handled || called != tc.called {
			t.Fatal("argument dispatch mismatch")
		}
		if !handled {
			continue
		}
		want := 2
		if called {
			want = 0
		}
		var result struct {
			Code   int    `json:"code"`
			Active bool   `json:"active"`
			State  string `json:"state"`
		}
		if code != want || json.Unmarshal(out.Bytes(), &result) != nil || result.Code != code || result.Active || result.State == "" {
			t.Fatal("receipt claimed active login support")
		}
		if called && tc.args[0] == "--prepare-login" && result.State != "configured" {
			t.Fatal("runtime-enabled configuration still reports pending", result.State)
		}
	}
}

func TestLoginConfigurationSanitizesFailureAndPreservesUncertainty(t *testing.T) {
	for _, cause := range []error{errors.New("private/profile/path"), errors.Join(inputworker.ErrLoginConfigurationUnconfirmed, errors.New("private/profile/path"))} {
		var out bytes.Buffer
		code, handled := runLoginConfiguration([]string{"--prepare-login", "B"}, &out, func(context.Context, string) error { return cause }, func(context.Context) error { return nil })
		want := 3
		if errors.Is(cause, inputworker.ErrLoginConfigurationUnconfirmed) {
			want = 5
		}
		if !handled || code != want || strings.Contains(out.String(), "private") {
			t.Fatal("failure lost uncertainty or leaked detail")
		}
	}
	if code, _ := runLoginConfiguration([]string{"--disable-login"}, brokenWriter{}, func(context.Context, string) error { return nil }, func(context.Context) error { return nil }); code != 6 {
		t.Fatal("unreadable receipt reported success")
	}
}
