package inputworker

import (
	"bytes"
	"encoding/binary"
)

const nativeLoginBootstrapSize = loginControlSize + 16

// This closed local bootstrap carries the service-activated login binding and
// two inherited identity references. No peer-supplied process/desktop selector
// is accepted; independent installation/service/grant checks remain mandatory.
func encodeNativeLoginBootstrap(binding loginBinding, service, parent uint64) ([nativeLoginBootstrapSize]byte, error) {
	var data [nativeLoginBootstrapSize]byte
	if service == 0 || parent == 0 || service == parent {
		return data, ErrAuthority
	}
	frame, err := encodeLoginControl(binding, loginActivate)
	if err != nil {
		return data, err
	}
	copy(data[:], frame[:])
	binary.BigEndian.PutUint64(data[loginControlSize:loginControlSize+8], service)
	binary.BigEndian.PutUint64(data[loginControlSize+8:], parent)
	return data, nil
}

func decodeNativeLoginBootstrap(data [nativeLoginBootstrapSize]byte) (loginBinding, []uint64, error) {
	binding, kind, err := readLoginControl(bytes.NewReader(data[:loginControlSize]))
	if err != nil || kind != loginActivate {
		return loginBinding{}, nil, ErrAuthority
	}
	service := binary.BigEndian.Uint64(data[loginControlSize : loginControlSize+8])
	parent := binary.BigEndian.Uint64(data[loginControlSize+8:])
	if _, err := encodeNativeLoginBootstrap(binding, service, parent); err != nil {
		return loginBinding{}, nil, err
	}
	return binding, []uint64{service, parent}, nil
}
