//go:build linux

package productcli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
)

func TestLinuxAutostartNeverRetriesDispatchedConnect(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/runtime" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(localapi.RuntimeInfo{
				SchemaVersion: localapi.RuntimeSchemaVersion, Protocol: localapi.AgentProtocol,
				Generation: localapi.AgentGeneration, Instance: strings.Repeat("a", 32), Build: "development",
			})
			return
		}
		requests.Add(1)
		// The agent may have applied CONNECT before its reply is lost.
		connection, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			connection.Close()
		}
	}))
	defer server.Close()
	base := t.TempDir()
	paths, err := productidentity.ResolveWindowsPaths(base)
	if err != nil {
		t.Fatal(err)
	}
	// PublishCredentials requires the already protected product root.
	if err := makeProtectedTestRoot(paths.Root); err != nil {
		t.Fatal(err)
	}
	session, err := localapi.PublishCredentials(paths.Control, strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var stdout, stderr bytes.Buffer
	code := RunWithAgentStartup(context.Background(), []string{"connect", "peer"}, &stdout, &stderr, base, func(context.Context) error {
		t.Error("a dispatched operation triggered autostart")
		return nil
	})
	if code != ExitAgentUnavailable || requests.Load() != 1 {
		t.Fatalf("lost reply: code=%d requests=%d stderr=%s", code, requests.Load(), stderr.String())
	}
}

func makeProtectedTestRoot(path string) error {
	// Keep this fixture independent of identity/certificate generation.
	if err := os.MkdirAll(filepath.Clean(path), 0o700); err != nil {
		return err
	}
	_, err := peercredentials.ProtectSecurityDirectory(path)
	return err
}
