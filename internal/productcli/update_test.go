package productcli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
)

func updateFixture(peer string, generation int) *stubClient {
	return &stubClient{
		trust: localapi.Trust{SchemaVersion: localapi.TrustSchemaVersion, Peers: []localapi.TrustedPeer{{Alias: "B", Fingerprint: peer}}},
		updateFn: func(r localapi.UpdateRequest) (localapi.UpdateResult, error) {
			switch r.Action {
			case "STATUS":
				return localapi.UpdateResult{Schema: localapi.UpdateSchema, Code: "ok", Status: peerupdate.Status{State: "IDLE", Allowed: []string{peer}}}, nil
			case "INSPECT":
				return localapi.UpdateResult{Schema: localapi.UpdateSchema, Code: "peer_status", PeerInfo: &peerupdate.PeerInspection{Peer: peer, State: "ONLINE", Generation: generation, Revision: strings.Repeat("a", 40), Allowed: true}}, nil
			case "SEND":
				return localapi.UpdateResult{Schema: localapi.UpdateSchema, Code: "queued", Status: peerupdate.Status{State: "QUEUED", Peer: peer, ID: strings.Repeat("c", 32)}}, nil
			}
			return localapi.UpdateResult{Schema: localapi.UpdateSchema, Code: "rejected"}, nil
		},
	}
}

func TestUpdateStatusShowsBothVersionsWithoutSending(t *testing.T) {
	peer := strings.Repeat("b", 64)
	client := updateFixture(peer, 76)
	var out, errs bytes.Buffer
	if code := run(context.Background(), []string{"update", "status", "B", "--json"}, &out, &errs, testDependencies(client)); code != ExitOK || !strings.Contains(out.String(), `"peerGeneration":76`) || !strings.Contains(out.String(), `"receiveAllowed":true`) || len(client.requests) != 2 || errs.Len() != 0 {
		t.Fatalf("status code/out/requests=%d %q %+v err=%q", code, out.String(), client.requests, errs.String())
	}
	for _, r := range client.requests {
		if r.Action == "SEND" {
			t.Fatal("status sent an update")
		}
	}
}

func TestUpdateSendSelectsCurrentOrTransitionBundle(t *testing.T) {
	root := t.TempDir()
	for _, generation := range []int{75, 77} {
		bundle := filepath.Join(root, fmt.Sprintf("g%d", generation), "peer-update")
		if err := os.MkdirAll(bundle, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bundle, "BUILD.json"), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	executable := filepath.Join(root, "g77", "peer-update", "krasis-cli.exe")
	peer := strings.Repeat("b", 64)
	for _, test := range []struct {
		generation int
		want       string
	}{
		{74, filepath.Join(root, "g75", "peer-update")},
		{76, filepath.Join(root, "g77", "peer-update")},
	} {
		client := updateFixture(peer, test.generation)
		deps := testDependencies(client)
		deps.executable = executable
		var out, errs bytes.Buffer
		if code := run(context.Background(), []string{"update", "send", "B", "--json"}, &out, &errs, deps); code != ExitOK || !strings.Contains(out.String(), `"code":"queued"`) || errs.Len() != 0 {
			t.Fatalf("send generation=%d code=%d out=%q err=%q", test.generation, code, out.String(), errs.String())
		}
		if got := client.requests[len(client.requests)-1]; got.Action != "SEND" || got.Directory != test.want || got.Peer != peer {
			t.Fatalf("wrong bundle generation=%d request=%+v", test.generation, got)
		}
	}
}

func TestUpdateSendUsesCurrentFlatPackageLayout(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "g79")
	transition := filepath.Join(root, "g75", "peer-update")
	for _, directory := range []string{current, transition} {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "BUILD.json"), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	executable := filepath.Join(current, "krasis-cli.exe")
	if got := currentBundleDirectory(executable, 78); got != current {
		t.Fatalf("flat bundle = %q, want %q", got, current)
	}
	if got := currentBundleDirectory(executable, 74); got != transition {
		t.Fatalf("transition bundle = %q, want %q", got, transition)
	}
}

func TestUpdateSendNeverSendsMissingBundleOrUnknownPeer(t *testing.T) {
	peer := strings.Repeat("b", 64)
	for _, test := range []struct {
		name string
		args []string
	}{
		{"missing bundle", []string{"update", "send", "B"}},
		{"unknown peer", []string{"update", "send", "other"}},
	} {
		client := updateFixture(peer, 76)
		deps := testDependencies(client)
		deps.executable = filepath.Join(t.TempDir(), "krasis-cli.exe")
		var out, errs bytes.Buffer
		if code := run(context.Background(), test.args, &out, &errs, deps); code != ExitUnsafeState {
			t.Fatalf("%s code=%d out=%q err=%q", test.name, code, out.String(), errs.String())
		}
		for _, r := range client.requests {
			if r.Action == "SEND" {
				t.Fatalf("%s unexpectedly sent update", test.name)
			}
		}
	}
}
