//go:build windows

package main

import (
	"bytes"
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
