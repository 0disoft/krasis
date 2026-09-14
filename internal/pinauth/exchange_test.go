package pinauth

import (
	"bytes"
	"net"
	"testing"
	"time"
)

func TestPINExchangeAndChannelBinding(t *testing.T) {
	for _, tc := range []struct {
		name, pin string
		mismatch  bool
		want      bool
	}{{"same", "012345", false, true}, {"wrong", "012346", false, false}, {"other-channel", "012345", true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			a, b := net.Pipe()
			defer a.Close()
			defer b.Close()
			a.SetDeadline(time.Now().Add(time.Second))
			b.SetDeadline(time.Now().Add(time.Second))
			binding := bytes.Repeat([]byte{1}, 32)
			other := append([]byte(nil), binding...)
			if tc.mismatch {
				other[0] = 2
			}
			result := make(chan error, 1)
			go func() { err := Exchange(a, "012345", binding, true); a.Close(); result <- err }()
			errB := Exchange(b, tc.pin, other, false)
			b.Close()
			errA := <-result
			if (errA == nil && errB == nil) != tc.want {
				t.Fatalf("A=%v B=%v", errA, errB)
			}
		})
	}
}

func TestMalformedAndInvalidPIN(t *testing.T) {
	for _, payload := range []string{"null", "{}", `{"Role":0}`, `{"Role":0,"Xᵤ":0,"Xᵥ":0}`} {
		var wire bytes.Buffer
		if err := write(&wire, []byte(payload)); err != nil {
			t.Fatal(err)
		}
		if Exchange(&wire, "012345", make([]byte, 32), false) == nil {
			t.Fatal("malformed peer accepted")
		}
	}
	for _, pin := range []string{"12345", "1234567", "abcdef", "１２３４５６"} {
		if Valid(pin) {
			t.Fatal("invalid PIN accepted")
		}
	}
	pin, err := Generate()
	if err != nil || !Valid(pin) {
		t.Fatal("PIN generation failed")
	}
}
