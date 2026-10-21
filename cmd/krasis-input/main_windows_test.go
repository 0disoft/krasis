//go:build windows

package main

import "testing"

func TestInputModesAreFixedServiceModes(t *testing.T) {
	for _, mode := range []string{"--service", "--input-worker", "--login-worker", "--native-input-worker", "--native-login-worker", "--native-release-worker", "--native-login-release-worker"} {
		if !validInputMode(mode) {
			t.Fatal(mode)
		}
	}
	for _, mode := range []string{"", "--desktop", "Winlogon", "--login-worker=1", "--login-worker --path=x"} {
		if validInputMode(mode) {
			t.Fatal("caller-selected mode admitted", mode)
		}
	}
}
