package inputworker

import (
	"errors"
	"strings"
	"syscall"
	"testing"
)

func TestConnectionFailurePreservesAuthorityAndRedactsNativeText(t *testing.T) {
	failure := connectionFailed("pipe_open", syscall.Errno(5))
	if !errors.Is(failure, ErrAuthority) || !errors.Is(failure, syscall.Errno(5)) {
		t.Fatal("connection failure lost authority or native error classification")
	}
	if ConnectionFailureEvent(failure) != "installed_input_stage_pipe_open" || ConnectionNativeFailureEvent(failure) != "installed_input_native_5" {
		t.Fatal("pipe-open denial lost its fixed diagnostic codes")
	}
	if ConnectionFailureEvent(connectionFailed("service_config", failure)) != "installed_input_stage_pipe_open" {
		t.Fatal("outer wrapper replaced the first failed stage")
	}
	secret := errors.New("private-path-and-identity")
	unknown := connectionFailed(secret.Error(), secret)
	if strings.Contains(unknown.Error(), "private") || ConnectionFailureEvent(unknown) != "installed_input_stage_unknown" || ConnectionNativeFailureEvent(unknown) != "installed_input_native_unknown" {
		t.Fatal("unbounded diagnostic text escaped")
	}
}

func TestSourceConnectionFailureRetainsSeparateStageAndNativeCause(t *testing.T) {
	failure := connectionFailed("pipe_open", syscall.Errno(2))
	if connectionFailureEvent(failure, "installed_source_stage_") != "installed_source_stage_pipe_open" ||
		connectionNativeFailureEvent(failure, "installed_source_native_") != "installed_source_native_2" {
		t.Fatal("source rebind failure lost its exact operation/native classification")
	}
	private := errors.New("private-path")
	if connectionFailureEvent(connectionFailed(private.Error(), private), "installed_source_stage_") != "installed_source_stage_unknown" ||
		connectionNativeFailureEvent(private, "installed_source_native_") != "installed_source_native_unknown" {
		t.Fatal("source diagnostics exposed unbounded error text")
	}
}
