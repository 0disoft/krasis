package wininput

import (
	"context"
	"errors"
)

// Source failures retain their original cause for recovery decisions. Only the
// closed stage vocabulary is exposed to support logs, never native error text.
type sourceStageError struct {
	stage string
	cause error
}

func (err *sourceStageError) Error() string { return err.stage + ": " + err.cause.Error() }
func (err *sourceStageError) Unwrap() error { return err.cause }

func sourceStage(stage string, err error) error {
	if err == nil {
		return nil
	}
	return &sourceStageError{stage: stage, cause: err}
}

func SourceFailureStage(err error) string {
	var stage *sourceStageError
	if !errors.As(err, &stage) {
		return ""
	}
	// Prefer the deepest operation over its containing worker.
	if inner := SourceFailureStage(stage.cause); inner != "" {
		return inner
	}
	switch stage.stage {
	case "low-level-hook", "raw-mouse", "wheel-capture", "thread-join",
		"hook-install", "hook-release", "raw-register", "raw-release",
		"raw-read", "raw-buffer", "raw-route", "message-loop", "pointer-snapshot", "cursor-present", "cursor-restore", "cursor-release", "raw-size", "raw-packet", "raw-desktop":
		return stage.stage
	}
	return ""
}

// A cursor snapshot fails before native hook/window owners are constructed.
// A joined restore/close error must never be hidden as that harmless startup race.
func SourcePointerSnapshotFailureOnly(err error) bool {
	if stage, ok := err.(*sourceStageError); ok {
		return stage.stage == "pointer-snapshot"
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !SourcePointerSnapshotFailureOnly(child) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return SourcePointerSnapshotFailureOnly(wrapped.Unwrap())
	}
	return false
}

func sourceWorkerStage(component string, err error) error {
	// Sibling cancellation must not obscure the worker that actually failed.
	if err == context.Canceled || err == context.DeadlineExceeded {
		return err
	}
	return sourceStage(component, err)
}
