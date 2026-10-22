package productpanel

import (
	"context"
	"reflect"
	"testing"
)

func TestDesktopLifecycleSaveOwnsPeerSnapshots(t *testing.T) {
	b := NewDesktopLifecycle(context.Background(), nil, nil, nil)
	peer := BridgePeer{Alias: "B", Fingerprint: "original"}
	input := BridgeSnapshot{Peers: []BridgePeer{peer}, InputRefreshCode: "ready", ReconnectCode: "ok"}
	result := b.save(input)

	input.Peers[0].Alias = "changed input"
	if result.Peers[0] != peer || b.Refresh().Peers[0] != peer {
		t.Fatal("input changed the returned or stored snapshot")
	}
	result.Peers[0].Fingerprint = "changed result"
	if input.Peers[0].Fingerprint != peer.Fingerprint || b.Refresh().Peers[0] != peer {
		t.Fatal("return value changed the input or stored snapshot")
	}
	view := b.Refresh()
	view.Peers[0].Alias = "changed refresh"
	if b.Refresh().Peers[0] != peer {
		t.Fatal("refresh changed the stored snapshot")
	}

	next := b.save(BridgeSnapshot{Peers: []BridgePeer{peer}})
	if result.Revision != 1 || next.Revision != 2 || next.InputRefreshCode != "ready" || next.ReconnectCode != "ok" {
		t.Fatal("save changed revision or maintenance notice behavior", next)
	}
	if !reflect.DeepEqual(next, b.Refresh()) {
		t.Fatal("returned and stored snapshots differ", next, b.Refresh())
	}
}

func TestDesktopLifecycleSavePreservesEmptyPeerRepresentation(t *testing.T) {
	for _, peers := range [][]BridgePeer{nil, {}} {
		b := NewDesktopLifecycle(context.Background(), nil, nil, nil)
		result := b.save(BridgeSnapshot{Peers: peers})
		if !reflect.DeepEqual(result.Peers, peers) || !reflect.DeepEqual(b.state.Peers, peers) {
			t.Fatal("save changed nil versus empty peers", result.Peers, peers)
		}
		if view := b.Refresh(); view.Peers == nil || len(view.Peers) != 0 {
			t.Fatal("refresh must expose an empty array", view.Peers)
		}
	}
}
