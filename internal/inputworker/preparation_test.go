package inputworker

import (
	"testing"
	"time"
)

func TestPreparationCannotDecodeAsLeaseOrCarryInput(t *testing.T) {
	now := time.Now()
	packet := encodePreparation()
	if !isPreparation(packet) {
		t.Fatal("preparation rejected")
	}
	if _, err := decodeLease(packet, now); err == nil {
		t.Fatal("preparation acquired lease authority")
	}
	for i := range packet {
		corrupt := packet
		corrupt[i] ^= 0x80
		if isPreparation(corrupt) {
			t.Fatalf("noncanonical preparation accepted at byte %d", i)
		}
	}
}

func TestPreparationCooldownDoesNotResetOnDuplicatesOrClockReversal(t *testing.T) {
	now := time.Now()
	for _, test := range []struct {
		last, now time.Time
		want      bool
	}{{time.Time{}, now, true}, {now, now.Add(time.Second), false},
		{now, now.Add(2 * time.Second), true}, {now, now.Add(-time.Second), false},
		{time.Time{}, time.Time{}, false}} {
		if preparationAllowed(test.last, test.now) != test.want {
			t.Fatal("preparation admission violated cooldown")
		}
	}
}
