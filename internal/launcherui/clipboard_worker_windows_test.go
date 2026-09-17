//go:build windows

package launcherui

import (
	"context"
	"testing"
)

func TestSharedClipboardPumpResetsWithoutConsentOrTrust(t *testing.T) {
	for _, peer := range []string{"", "untrusted"} {
		p := clipboardPump{peer: "old", sequence: 42}
		// Invalid owner/control are deliberate: these cases must return before any
		// native clipboard access or local-agent request.
		p.sync(context.Background(), 0, "invalid-control", peer, nil)
		if p.peer != "" || p.sequence != 0 {
			t.Fatal(p)
		}
	}
}
