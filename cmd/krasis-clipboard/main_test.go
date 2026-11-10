package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestCLIHelpAndInvalidArgumentsDoNotStartBridge(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
	}{
		{[]string{"--help"}, 0}, {nil, 2}, {[]string{"--control-dir", "dir", "unexpected"}, 2},
		{[]string{"--control-dir", "dir", "--poll-interval", "0s"}, 2},
		{[]string{"--control-dir", "dir", "--poll-interval", "2s"}, 2},
	} {
		var out bytes.Buffer
		if code := run(context.Background(), tc.args, &out); code != tc.code || !strings.Contains(out.String(), "usage:") {
			t.Fatalf("%v: %d %s", tc.args, code, out.String())
		}
	}
}
