package peerupdate

import (
	"github.com/0disoft/krasis/internal/keyboardstate"
	"strings"
	"testing"
	"time"
)

func TestPeerKeyboardExpiresUsingBothObservationAndSampleAge(t *testing.T) {
	now := time.Unix(100, 0)
	state := keyboardstate.State{Caps: "ON", Num: "OFF", Hangul: "UNKNOWN"}
	m := &Manager{peers: map[string]peerObservation{"peer": {at: now, info: PeerInfo{Keyboard: &state, KeyboardAgeMillis: 3000}}}}
	if got := m.PeerKeyboard("peer", now.Add(2*time.Second)); got != state {
		t.Fatal(got)
	}
	if got := m.PeerKeyboard("peer", now.Add(3*time.Second)); got != keyboardstate.Unknown() {
		t.Fatal("stale sample refreshed by transport", got)
	}
	if got := m.PeerKeyboard("absent", now); got != keyboardstate.Unknown() {
		t.Fatal("missing peer became known")
	}
}

func TestPeerKeyboardProtocolRejectsMalformedModesAndAcceptsLegacy(t *testing.T) {
	for _, tc := range []struct {
		extra string
		valid bool
	}{
		{"", true},
		{`,"keyboard":{"caps":"ON","num":"OFF","hangul":"UNKNOWN"}`, true},
		{`,"keyboard":{"caps":"ON","num":"password","hangul":"UNKNOWN"}`, false},
		{`,"keyboardAgeMillis":6001`, false},
		{`,"keyboardAgeMillis":1`, false},
	} {
		_, err := readPeerInfo(strings.NewReader(`{"protocol":2,"generation":205` + tc.extra + `}`))
		if (err == nil) != tc.valid {
			t.Fatal(tc, err)
		}
	}
}
