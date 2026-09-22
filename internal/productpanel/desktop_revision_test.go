package productpanel

import (
	"context"
	"testing"
)

func TestNativeProjectionRevisionsAdvanceWithoutMutatingOlderSnapshots(t *testing.T) {
	b := NewDesktopLifecycle(context.Background(), nil, nil, nil)
	first := b.save(BridgeSnapshot{State: "RECONNECTING"})
	second := b.save(BridgeSnapshot{State: "CONNECTED", Revision: 999})
	if first.Revision != 1 || second.Revision != 2 || b.Refresh().Revision != 2 || first.State != "RECONNECTING" {
		t.Fatalf("revision drift: %+v %+v", first, second)
	}
}
