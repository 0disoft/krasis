package transport

import (
	"encoding/binary"
	"github.com/0disoft/krasis/internal/control"
	"testing"
)

func TestCursorHeightPayloadRoundTrip(t *testing.T) {
	for _, kind := range []ControlCommandKind{ControlLeaseRequest, ControlLeaseRevoke} {
		for _, h := range []control.EdgeHeight{0, 1, 32768, 32769, 65536} {
			c := ControlCommand{Kind: kind, SessionID: "s", Epoch: "e", Height: h}
			if kind == ControlLeaseRequest {
				c.Edge = control.ScreenEdgeRight
			} else {
				c.LeaseID = "l"
			}
			p, err := EncodeControlPayload(c)
			if err != nil {
				t.Fatal(err)
			}
			d, err := DecodeControlPayload(p)
			if err != nil || d != c {
				t.Fatalf("%+v got %+v err=%v", c, d, err)
			}
			if h == 0 && p[0] != 3 || h != 0 && p[0] != 5 {
				t.Fatal("lost legacy encoding")
			}
			if h == 0 {
				continue
			}
			for _, bad := range []uint32{0, 65537, ^uint32(0)} {
				copyP := append([]byte(nil), p...)
				binary.BigEndian.PutUint32(copyP[8:12], bad)
				if _, err := DecodeControlPayload(copyP); err == nil {
					t.Fatal("accepted invalid height", bad)
				}
			}
			for n := 8; n < 12; n++ {
				if _, err := DecodeControlPayload(p[:n]); err == nil {
					t.Fatal("accepted truncation")
				}
			}
		}
	}
	for _, kind := range []ControlCommandKind{ControlPrepareInput, ControlLeaseGrant, ControlLeaseCommit, ControlLeaseAbort} {
		if _, err := EncodeControlPayload(ControlCommand{Kind: kind, SessionID: "s", LeaseID: "l", Epoch: "e", Height: 1}); err == nil {
			t.Fatal("height accepted on unrelated command")
		}
	}
}

func TestCursorHeightNegotiationWithOlderPeers(t *testing.T) {
	for mode := 0; mode < 3; mode++ {
		f := newTLSFixture(t)
		sc := mustServerTLSConfig(t, f.serverIdentity, f.roots)
		cc := mustClientTLSConfig(t, f.clientIdentity, f.roots, f.serverName)
		EnableCursorHeight(sc)
		if mode == 1 {
			EnableInputPreparation(cc)
		}
		if mode == 2 {
			EnableCursorHeight(cc)
		}
		s, c := openTLSPair(t, sc, cc)
		server, client, se, ce := authenticateConnectionPair(t, s, c)
		if se != nil || ce != nil {
			t.Fatal(se, ce)
		}
		if server.SupportsCursorHeight() != (mode == 2) || client.SupportsCursorHeight() != (mode == 2) {
			t.Fatal("height negotiation mismatch")
		}
		if server.SupportsInputPreparation() != (mode > 0) {
			t.Fatal("preparation fallback lost")
		}
	}
}
