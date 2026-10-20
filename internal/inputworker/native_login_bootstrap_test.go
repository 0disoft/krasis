package inputworker

import (
	"encoding/binary"
	"testing"
)

func TestNativeLoginBootstrapKeepsActivationAndDistinctReferences(t *testing.T) {
	binding := loginBinding{session: 1, profile: [32]byte{2}, nonce: [32]byte{3}}
	data, err := encodeNativeLoginBootstrap(binding, 11, 22)
	if err != nil {
		t.Fatal(err)
	}
	actual, handles, err := decodeNativeLoginBootstrap(data)
	if err != nil || actual != binding || len(handles) != 2 || handles[0] != 11 || handles[1] != 22 {
		t.Fatal(actual, handles, err)
	}
	for _, mutate := range []func(*[nativeLoginBootstrapSize]byte){
		func(d *[nativeLoginBootstrapSize]byte) { d[0] = 'X' },
		func(d *[nativeLoginBootstrapSize]byte) { d[5] = byte(loginReady) },
		func(d *[nativeLoginBootstrapSize]byte) { d[6] = 1 },
		func(d *[nativeLoginBootstrapSize]byte) { binary.BigEndian.PutUint32(d[8:12], 0) },
		func(d *[nativeLoginBootstrapSize]byte) { d[12] = 0 },
		func(d *[nativeLoginBootstrapSize]byte) { d[44] = 0 },
		func(d *[nativeLoginBootstrapSize]byte) { binary.BigEndian.PutUint64(d[loginControlSize:], 0) },
		func(d *[nativeLoginBootstrapSize]byte) { binary.BigEndian.PutUint64(d[loginControlSize+8:], 11) },
	} {
		broken := data
		mutate(&broken)
		if _, _, err := decodeNativeLoginBootstrap(broken); err == nil {
			t.Fatal("malformed login activation bootstrap admitted")
		}
	}
}
