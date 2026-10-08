//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"github.com/0disoft/krasis/internal/appinstall"
	"testing"
)

func TestGenericSetupCannotInstallOrAcceptCustomPaths(t *testing.T) {
	for _, args := range [][]string{{"--install", "C:\\Windows"}, {"--remove", "--root"}, {"--verify"}} {
		var out bytes.Buffer
		if code := run(append(args, "--json"), &out); code == 0 {
			t.Fatal("unbuilt or unbounded setup accepted", args)
		}
	}
}

func TestSetupKeepsLaunchFailureDistinctFromInstallationAndShutdownFailure(t *testing.T) {
	var out bytes.Buffer
	if code := result(&out, true, "installed_launch_failed", appinstall.ErrInvalid); code != 1 {
		t.Fatal(code)
	}
	var got struct {
		State         string
		Code          int
		ShutdownStage string
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || got.State != "installed_launch_failed" || got.Code != 1 || got.ShutdownStage != "" {
		t.Fatal(out.String(), err)
	}
}
