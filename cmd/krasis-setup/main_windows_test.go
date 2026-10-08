//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/0disoft/krasis/internal/appinstall"
	"golang.org/x/sys/windows"
	"os"
	"strings"
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

func TestSetupFailureJSONKeepsStageAndKnownNativeErrorWithoutPrivateDetails(t *testing.T) {
	_, failed := appinstall.Install(t.TempDir(), "unused.exe", appinstall.Package{})
	for _, native := range []bool{false, true} {
		err := failed
		if native {
			err = errors.Join(err, &os.PathError{Op: "open", Path: `C:\private-user\secret-name.exe`, Err: windows.ERROR_ACCESS_DENIED})
		}
		var out bytes.Buffer
		if code := result(&out, true, "install_failed", err); code != 1 {
			t.Fatal(code)
		}
		var got struct {
			State        string
			InstallStage string
			Win32Error   *uint32
		}
		if err := json.Unmarshal(out.Bytes(), &got); err != nil || got.State != "install_failed" || got.InstallStage != "preflight" {
			t.Fatal("failure stage lost", out.String(), err)
		}
		if native && (got.Win32Error == nil || *got.Win32Error != 5) || !native && got.Win32Error != nil {
			t.Fatal("native error invented or discarded", out.String())
		}
		if strings.Contains(out.String(), "private-user") || strings.Contains(out.String(), "secret-name") || strings.Contains(out.String(), "invalid application") {
			t.Fatal("raw failure leaked into result", out.String())
		}
	}
	var out bytes.Buffer
	if code := result(&out, true, "installed", nil); code != 0 || strings.Contains(out.String(), "installStage") || strings.Contains(out.String(), "win32Error") {
		t.Fatal("success contains fabricated failure", out.String())
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
