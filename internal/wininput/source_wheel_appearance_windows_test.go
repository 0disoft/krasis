//go:build windows

package wininput

import (
	"errors"
	"reflect"
	"syscall"
	"testing"
)

func TestSourceWheelCaptureAppearancePreservesHitTesting(t *testing.T) {
	previous := callSetLayeredSourceCapture
	t.Cleanup(func() { callSetLayeredSourceCapture = previous })
	callSetLayeredSourceCapture = func(args ...uintptr) (uintptr, uintptr, error) {
		// Nonzero minimum alpha with LWA_ALPHA only: no click-through color key.
		if want := []uintptr{42, 0, 1, 2}; !reflect.DeepEqual(args, want) {
			t.Fatalf("layered window arguments = %v, want %v", args, want)
		}
		return 1, 0, syscall.Errno(0)
	}
	if err := configureSourceWheelCaptureAppearance(42); err != nil {
		t.Fatal(err)
	}
}

func TestSourceWheelCaptureAppearanceFailureStopsSetup(t *testing.T) {
	previous := callSetLayeredSourceCapture
	t.Cleanup(func() { callSetLayeredSourceCapture = previous })
	for _, failure := range []error{syscall.Errno(5), syscall.Errno(0)} {
		callSetLayeredSourceCapture = func(...uintptr) (uintptr, uintptr, error) {
			return 0, 0, failure
		}
		err := configureSourceWheelCaptureAppearance(42)
		if !errors.Is(err, ErrSourceCaptureActivationSetup) {
			t.Fatalf("appearance failure = %v, want capture setup failure", err)
		}
		var native syscall.Errno
		if failure != syscall.Errno(0) && (!errors.As(err, &native) || native != failure) {
			t.Fatalf("original native failure was erased: %v", err)
		}
	}
}
