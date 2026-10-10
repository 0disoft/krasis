package loginagent

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/transport"
)

func testGrant(now time.Time) Grant {
	return Grant{Schema: GrantSchema, ID: strings.Repeat("1", 64), Owner: "S-1-5-21-1-2-3-1001", Alias: "B",
		Peer: strings.Repeat("2", 64), Role: transport.AuthenticatedPeerListener, Identity: strings.Repeat("3", 64),
		LocalTransport: strings.Repeat("4", 64), AllowInbound: true, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
}

func TestGrantRejectsAmbiguousPolicyAndHasNoPrivatePayload(t *testing.T) {
	now := time.Now().UTC()
	g := testGrant(now)
	data, err := EncodeGrant(g, now)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeGrant(data, now)
	if err != nil || decoded != g {
		t.Fatal("grant roundtrip", err)
	}
	first, err := g.Selection(now)
	if err != nil {
		t.Fatal(err)
	}
	g.ID = strings.Repeat("5", 64)
	second, err := g.Selection(now)
	if err != nil || first.Profile == second.Profile {
		t.Fatal("reprovision reused lifetime key")
	}
	for _, data := range [][]byte{
		[]byte("null"), []byte("{}"), append(append([]byte(nil), data...), []byte(" {}")...),
		bytes.Replace(data, []byte(`"allowInboundControl":true`), []byte(`"allowInboundControl":true,"allowInboundControl":false`), 1),
		bytes.Replace(data, []byte(`"allowInboundControl":true`), []byte(`"allowInboundControl":null`), 1),
		bytes.Replace(data, []byte(`"allowInboundControl":true,`), nil, 1),
		append([]byte(`{"command":"cmd.exe",`), data[1:]...),
		append([]byte(`{"privateKey":"not-allowed",`), data[1:]...),
		append([]byte(`{"path":"C:\\untrusted",`), data[1:]...),
		bytes.Repeat([]byte("x"), 4097),
	} {
		if _, err := DecodeGrant(data, now); err == nil {
			t.Fatal("ambiguous/extra policy accepted")
		}
	}
}

func TestGrantExpiryAndPrincipalBoundaries(t *testing.T) {
	now := time.Now().UTC()
	for _, change := range []func(*Grant){
		func(g *Grant) { g.Owner = "S-1-5-18" },
		func(g *Grant) { g.Owner = "S-1-5-32-544" },
		func(g *Grant) { g.Owner = "S-1-5-21-01-2-3-1001" },
		func(g *Grant) { g.Owner = "S-1-5-21-1-2-3-4294967296" },
		func(g *Grant) { g.CreatedAt = now.Add(time.Second) },
		func(g *Grant) { g.ExpiresAt = now },
		func(g *Grant) { g.ExpiresAt = now.Add(31 * 24 * time.Hour) },
		func(g *Grant) { g.Role = "OTHER" },
		func(g *Grant) { g.Peer = strings.Repeat("0", 64) },
		func(g *Grant) { g.Identity = "" },
	} {
		g := testGrant(now)
		change(&g)
		if g.Validate(now) == nil {
			t.Fatal("invalid grant accepted")
		}
	}
	g := testGrant(now)
	g.Owner = "S-1-12-1-1-2-3-4"
	if g.Validate(now) != nil {
		t.Fatal("Entra account rejected")
	}
}
