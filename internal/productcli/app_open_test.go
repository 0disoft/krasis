package productcli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
)

type appOpenClient struct {
	stubClient
	calls int
}

func (c *appOpenClient) OpenApp(context.Context, localapi.AppOpenRequest) (localapi.AppOpenResult, error) {
	c.calls++
	return localapi.AppOpenResult{Code: "launch_requested"}, nil
}

func TestCLIAppOpenStatusAndUnknownPeer(t *testing.T) {
	client := &appOpenClient{stubClient: stubClient{trust: localapi.Trust{SchemaVersion: localapi.TrustSchemaVersion, Peers: []localapi.TrustedPeer{{Alias: "B", Fingerprint: strings.Repeat("a", 64)}}}}}
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"app", "open-status", "B", strings.Repeat("b", 32), "--json"}, &out, &errOut, testDependencies(client)); code != ExitOK || !strings.Contains(out.String(), "launch_requested") || client.calls != 1 {
		t.Fatalf("code=%d out=%s", code, out.String())
	}
	if code := run(context.Background(), []string{"app", "open", "unknown", "--json"}, &out, &errOut, testDependencies(client)); code != ExitUnsafeState || client.calls != 1 {
		t.Fatal("unknown peer called API")
	}
	if code := run(context.Background(), []string{"app", "open", "B", "other.exe"}, &out, &errOut, testDependencies(client)); code != ExitUsage || client.calls != 1 {
		t.Fatal("path argument accepted")
	}
}
