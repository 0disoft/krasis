package loginagent

import (
	"bytes"
	"testing"
	"time"
)

func TestPersistentGrantHasNoRecurringExpiryButLegacyDoes(t *testing.T) {
	now := time.Now().UTC()
	old := testGrant(now)
	g := old
	g.Schema, g.ExpiresAt = PersistentGrantSchema, time.Time{}
	data, err := EncodeGrant(g, now)
	if err != nil || bytes.Contains(data, []byte("expiresAt")) {
		t.Fatalf("persistent encoding: %s %v", data, err)
	}
	for _, at := range []time.Time{now, now.AddDate(3, 0, 0)} {
		got, err := DecodeGrant(data, at)
		if err != nil || got != g {
			t.Fatal("persistent grant expired", err)
		}
	}
	if old.Validate(now.AddDate(3, 0, 0)) == nil {
		t.Fatal("legacy bounded consent silently extended")
	}
	g.ExpiresAt = now.Add(time.Hour)
	if g.Validate(now) == nil {
		t.Fatal("mixed persistent/expiry policy accepted")
	}
}
