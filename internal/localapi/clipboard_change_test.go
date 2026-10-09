package localapi

import (
	"strings"
	"testing"
)

func TestContentFreeLocalChangeIsSyncOnly(t *testing.T) {
	peer := strings.Repeat("a", 64)
	if !(ClipboardRequest{Action: "SYNC", Peer: peer, LocalChanged: true}).Valid() {
		t.Fatal("content-free change rejected")
	}
	for _, action := range []string{"ENABLE", "DISABLE", "REPORT"} {
		if (ClipboardRequest{Action: action, Peer: peer, LocalChanged: true}).Valid() {
			t.Fatal(action)
		}
	}
	if (ClipboardRequest{Action: "SYNC", Peer: peer, LocalChanged: true, Ack: strings.Repeat("b", 32)}).Valid() {
		t.Fatal("change and application acknowledgement conflated")
	}
}
