package productcli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
)

func TestClipboardSwitchUsesAgentAcknowledgmentAndReadback(t *testing.T) {
	peer := strings.Repeat("b", 64)
	current := ""
	client := updateFixture(peer, 77)
	client.updateFn = func(r localapi.UpdateRequest) (localapi.UpdateResult, error) {
		return localapi.UpdateResult{Schema: localapi.UpdateSchema, Code: "ok", Status: peerupdate.Status{State: "IDLE", ClipboardPeer: current}}, nil
	}
	var writes []localapi.ClipboardRequest
	client.clipboardFn = func(r localapi.ClipboardRequest) (localapi.ClipboardResult, error) {
		writes = append(writes, r)
		if r.Action == "ENABLE" {
			current = r.Peer
		} else {
			current = ""
		}
		return localapi.ClipboardResult{Code: "ok"}, nil
	}
	for _, test := range []struct {
		args  []string
		want  string
		calls int
	}{
		{[]string{"clipboard", "status", "B", "--json"}, `"enabled":false`, 0},
		{[]string{"clipboard", "on", "B", "--json"}, `"enabled":true`, 1},
		{[]string{"clipboard", "on", "B", "--json"}, `"enabled":true`, 1},
		{[]string{"clipboard", "off", "B", "--json"}, `"enabled":false`, 2},
	} {
		var out, errs bytes.Buffer
		if code := run(context.Background(), test.args, &out, &errs, testDependencies(client)); code != ExitOK || !strings.Contains(out.String(), test.want) || len(writes) != test.calls || errs.Len() != 0 {
			t.Fatalf("%v code=%d out=%q writes=%+v err=%q", test.args, code, out.String(), writes, errs.String())
		}
	}
	if writes[0].Action != "ENABLE" || writes[1].Action != "DISABLE" || writes[0].Peer != peer || writes[1].Peer != peer {
		t.Fatalf("unexpected actions: %+v", writes)
	}
}

func TestClipboardOffDoesNotDisableAnotherPeer(t *testing.T) {
	peer := strings.Repeat("b", 64)
	client := updateFixture(peer, 77)
	client.updateFn = func(localapi.UpdateRequest) (localapi.UpdateResult, error) {
		return localapi.UpdateResult{Schema: localapi.UpdateSchema, Code: "ok", Status: peerupdate.Status{State: "IDLE", ClipboardPeer: strings.Repeat("c", 64)}}, nil
	}
	client.clipboardFn = func(localapi.ClipboardRequest) (localapi.ClipboardResult, error) {
		t.Fatal("clipboard mutation should not reach agent")
		return localapi.ClipboardResult{}, nil
	}
	var out, errs bytes.Buffer
	if code := run(context.Background(), []string{"clipboard", "off", "B", "--json"}, &out, &errs, testDependencies(client)); code != ExitUnsafeState || !strings.Contains(out.String(), `"otherPeerActive":true`) {
		t.Fatalf("off another peer code=%d out=%q err=%q", code, out.String(), errs.String())
	}
}
