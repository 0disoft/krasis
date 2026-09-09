package productcli

import (
	"bytes"
	"context"
	"testing"
)

func TestInvalidConnectionNeverOpensAgent(t *testing.T) {
	for _, args := range [][]string{
		{"connection"}, {"connection", "show"},
		{"connection", "set", "peer", "8.8.8.8:1234", "RIGHT"},
		{"connection", "set", "peer", "192.168.1.2:1234", "TOP"},
		{"connection", "set", "peer", "example.com:1234", "LEFT"},
	} {
		deps := dependencies{openClient: func(string) (apiClient, error) { t.Fatal("opened agent for invalid arguments"); return nil, nil }}
		var stdout, stderr bytes.Buffer
		if code := run(context.Background(), args, &stdout, &stderr, deps); code != ExitUsage {
			t.Fatalf("%v: exit %d", args, code)
		}
	}
}
