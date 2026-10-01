//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/inputworker"
	"github.com/0disoft/krasis/internal/installationdiag"
)

func TestInstallerArgumentsNeverSelectExternalPayload(t *testing.T) {
	for _, args := range [][]string{nil, {"--install", "other.exe"}, {"--remove", "--install"}, {"--payload=x"}, {"--help"}} {
		var out bytes.Buffer
		load := func() (installerPayload, error) {
			t.Fatal("non-operation loaded payload")
			return installerPayload{}, nil
		}
		apply := func(context.Context, bool, []byte, []byte, [32]byte, [32]byte) inputworker.InstallationResult {
			t.Fatal("non-operation changed machine")
			return 0
		}
		want := 2
		if len(args) == 1 && args[0] == "--help" {
			want = 0
		}
		if code := run(args, &out, load, apply); code != want {
			t.Fatal("argument exit code mismatch")
		}
	}
}

func TestInstallerReceiptsAndBoundedDispatch(t *testing.T) {
	for _, remove := range []bool{false, true} {
		for _, result := range []inputworker.InstallationResult{0, 3, 4, 5, 255} {
			var out bytes.Buffer
			load := func() (installerPayload, error) {
				return installerPayload{agent: []byte{1}, input: []byte{2}, agentPin: [32]byte{3}, inputPin: [32]byte{4}}, nil
			}
			apply := func(ctx context.Context, actualRemove bool, agent, input []byte, agentPin, inputPin [32]byte) inputworker.InstallationResult {
				if _, ok := ctx.Deadline(); !ok || ctx.Err() != nil || actualRemove != remove || !bytes.Equal(agent, []byte{1}) || !bytes.Equal(input, []byte{2}) || agentPin != ([32]byte{3}) || inputPin != ([32]byte{4}) {
					t.Fatal("dispatch lost deadline or compiled payload")
				}
				return result
			}
			arg := "--install"
			if remove {
				arg = "--remove"
			}
			want := int(result)
			if result == 255 {
				want = 5
			}
			if code := run([]string{arg}, &out, load, apply); code != want {
				t.Fatal("result exit code mismatch")
			}
			var receipt struct {
				Schema string `json:"schemaVersion"`
				Code   int    `json:"code"`
				State  string `json:"state"`
			}
			if json.Unmarshal(out.Bytes(), &receipt) != nil || receipt.Schema != "krasis.input.install-result/v1" || receipt.Code != want || receipt.State == "" {
				t.Fatal("missing machine-readable receipt")
			}
		}
	}
}

func TestInstallerLoadErrorIsSanitizedAndOutputFailureIsDistinct(t *testing.T) {
	load := func() (installerPayload, error) { return installerPayload{}, errors.New("private/path") }
	apply := func(context.Context, bool, []byte, []byte, [32]byte, [32]byte) inputworker.InstallationResult {
		t.Fatal("failed payload applied")
		return 0
	}
	var out bytes.Buffer
	if run([]string{"--install"}, &out, load, apply) != 3 || strings.Contains(out.String(), "private") {
		t.Fatal("payload failure leaked detail")
	}
	if run([]string{"--install"}, brokenWriter{}, load, apply) != 6 {
		t.Fatal("unreadable receipt reported as success")
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestDetailedInstallerPrintsReceiptEvenIfDiagnosticSaveFails(t *testing.T) {
	s := installationdiag.Summary{Time: "2026-10-02T00:00:00Z", Action: "install", Result: 5, Stage: "stage", Operation: "file_open", Win32Error: 5, Win32Known: true, Recovery: "required"}
	var out bytes.Buffer
	code := runDetailed([]string{"--install"}, &out, func() (installerPayload, error) { return installerPayload{}, nil },
		func(context.Context, bool, []byte, []byte, [32]byte, [32]byte) inputworker.InstallationReport {
			return inputworker.InstallationReport{Result: inputworker.InstallationRecoveryRequired, Diagnostic: s}
		},
		func(installationdiag.Summary) error { return errors.New("PRIVATE-PATH") })
	if code != 5 || !strings.Contains(out.String(), `"win32Error":5`) || !strings.Contains(out.String(), `"diagnosticSaved":false`) || strings.Contains(out.String(), "PRIVATE") {
		t.Fatal("installer lost or leaked receipt")
	}
}
