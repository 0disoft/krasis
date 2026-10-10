//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/0disoft/krasis/internal/inputworker"
	"github.com/0disoft/krasis/internal/logindiag"
)

func runLoginConfiguration(args []string, out io.Writer, configure func(context.Context, string) error, remove func(context.Context) error) (int, bool) {
	return runLoginConfigurationTracked(args, out, configure, remove, nil)
}

func runLoginConfigurationTracked(args []string, out io.Writer, configure func(context.Context, string) error, remove func(context.Context) error, save func(logindiag.Summary) error) (int, bool) {
	if len(args) == 0 || args[0] != "--prepare-login" && args[0] != "--disable-login" {
		return 0, false
	}
	code, state := 2, "invalid_arguments"
	var cause error
	valid := len(args) == 2 && args[0] == "--prepare-login" && strings.TrimSpace(args[1]) == args[1] && args[1] != "" && len(args[1]) <= 256 && !strings.HasPrefix(args[1], "--")
	valid = valid || len(args) == 1 && args[0] == "--disable-login"
	if valid && configure != nil && remove != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		var err error
		if args[0] == "--disable-login" {
			err = remove(ctx)
		} else {
			err = configure(ctx, args[1])
		}
		cancel()
		cause = err
		code, state = 3, "configuration_failed"
		if errors.Is(err, inputworker.ErrLoginConfigurationUnconfirmed) {
			code, state = 5, "configuration_unconfirmed"
		}
		if err == nil {
			code, state = 0, "configured"
			if args[0] == "--disable-login" {
				state = "disabled"
			}
		}
	}
	action := "prepare"
	if args[0] == "--disable-login" {
		action = "disable"
	}
	diagnostic := logindiag.New(action, code, cause)
	var saved *bool
	if save != nil {
		ok := save(diagnostic) == nil
		saved = &ok
	}
	if json.NewEncoder(out).Encode(struct {
		Schema          string            `json:"schemaVersion"`
		Code            int               `json:"code"`
		State           string            `json:"state"`
		Active          bool              `json:"active"`
		Diagnostic      logindiag.Summary `json:"diagnostic"`
		DiagnosticSaved *bool             `json:"diagnosticSaved,omitempty"`
	}{"krasis.login-configuration-result/v1", code, state, false, diagnostic, saved}) != nil {
		return 6, true
	}
	return code, true
}
