// Package windowsscript preserves bounded, numeric diagnostics from Windows
// setup helpers without exposing their environment, paths or exception text.
package windowsscript

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Failure retains the process result for errors.As and the caller's deadline
// for errors.Is. Untrusted helper output is never part of its error string.
type Failure struct {
	Operation string
	ExitCode  int
	Stage     int
	HRESULT   int32
	Line      int
	Timeout   bool
	cause     error
}

func (e *Failure) Error() string {
	detail := "diagnostic=unavailable"
	if e.Stage != 0 {
		detail = fmt.Sprintf("stage=%d hresult=%d line=%d", e.Stage, e.HRESULT, e.Line)
	}
	return fmt.Sprintf("%s failed (exit=%d timeout=%t %s)", e.Operation, e.ExitCode, e.Timeout, detail)
}

func (e *Failure) Unwrap() error { return e.cause }

// Run owns stdout/stderr and waits for the helper to exit. It does not retry a
// possibly applied setup operation. Scripts emit only the closed numeric form
// KRASIS_HELPER_ERROR stage=N hresult=N line=N on a caught exception.
func Run(ctx context.Context, command *exec.Cmd, operation string) error {
	var output boundedOutput
	command.Stdout, command.Stderr = &output, &output
	if err := command.Run(); err != nil {
		failure := &Failure{Operation: operation, ExitCode: -1, Timeout: errors.Is(ctx.Err(), context.DeadlineExceeded), cause: errors.Join(err, ctx.Err())}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			failure.ExitCode = exit.ExitCode()
		}
		for _, line := range strings.Split(string(output.data), "\n") {
			fields := strings.Fields(line)
			if len(fields) != 4 || fields[0] != "KRASIS_HELPER_ERROR" {
				continue
			}
			stage, stageErr := numericField(fields[1], "stage=", 8)
			hresult, resultErr := numericField(fields[2], "hresult=", 32)
			location, lineErr := numericField(fields[3], "line=", 32)
			if stageErr == nil && resultErr == nil && lineErr == nil && stage > 0 && stage <= 16 && location > 0 && location <= 10000 {
				failure.Stage, failure.HRESULT, failure.Line = int(stage), int32(hresult), int(location)
			}
		}
		return failure
	}
	return nil
}

func numericField(field, prefix string, bits int) (int64, error) {
	if !strings.HasPrefix(field, prefix) {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseInt(strings.TrimPrefix(field, prefix), 10, bits)
}

type boundedOutput struct{ data []byte }

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := 4096 - len(b.data); remaining > 0 {
		b.data = append(b.data, p[:min(remaining, n)]...)
	}
	return n, nil
}
