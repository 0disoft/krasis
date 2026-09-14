// Package pinauth authenticates a short, one-use PIN on an existing TLS channel.
package pinauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/schollz/pake/v3"
)

var ErrAuthentication = errors.New("PIN authentication failed")

func Valid(pin string) bool {
	if len(pin) != 6 {
		return false
	}
	for _, c := range pin {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func Generate() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// Exchange proves knowledge in both directions, binding the result to the exact
// TLS exporter. It never sends the PIN or a PIN hash. The caller owns deadlines.
func Exchange(wire io.ReadWriter, pin string, binding []byte, initiator bool) (err error) {
	// The dependency's point decoder can panic on malformed/null public values.
	// Reject the exchange, never continue with an unverified or partial key.
	defer func() {
		if recover() != nil {
			err = ErrAuthentication
		}
	}()
	if !Valid(pin) || len(binding) != 32 {
		return ErrAuthentication
	}
	role := 1
	if initiator {
		role = 0
	}
	password := []byte(pin)
	defer clear(password)
	p, err := pake.InitCurve(password, role, "p256")
	if err != nil {
		return ErrAuthentication
	}
	update := func() error {
		data, err := read(wire)
		if err != nil {
			return err
		}
		var public struct{ Role *int }
		if json.Unmarshal(data, &public) != nil || public.Role == nil || *public.Role != 1-role {
			return ErrAuthentication
		}
		return p.Update(data)
	}
	if initiator {
		if err = write(wire, p.Bytes()); err != nil {
			return err
		}
		if err = update(); err != nil {
			return ErrAuthentication
		}
	} else {
		if err = update(); err != nil {
			return ErrAuthentication
		}
		if err = write(wire, p.Bytes()); err != nil {
			return err
		}
	}
	key, err := p.SessionKey()
	if err != nil || len(key) != 32 {
		return ErrAuthentication
	}
	defer clear(key)
	proof := func(role string) []byte {
		h := hmac.New(sha256.New, key)
		h.Write([]byte("krasis-pin-p256-v1/" + role))
		h.Write(binding)
		return h.Sum(nil)
	}
	verify := func(role string) error {
		received, err := read(wire)
		if err != nil {
			return err
		}
		if !hmac.Equal(received, proof(role)) {
			return ErrAuthentication
		}
		return nil
	}
	if initiator {
		if err = write(wire, proof("inviter")); err != nil {
			return err
		}
		return verify("joiner")
	}
	if err = verify("inviter"); err != nil {
		return err
	}
	return write(wire, proof("joiner"))
}

func write(w io.Writer, data []byte) error {
	if len(data) == 0 || len(data) > 4096 {
		return ErrAuthentication
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(data)))
	frame := append(header[:], data...)
	for len(frame) > 0 {
		n, err := w.Write(frame)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrNoProgress
		}
		frame = frame[n:]
	}
	return nil
}
func read(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(header[:])
	if n == 0 || n > 4096 {
		return nil, ErrAuthentication
	}
	data := make([]byte, n)
	_, err := io.ReadFull(r, data)
	return data, err
}
