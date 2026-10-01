//go:build windows && inputinstaller

package main

import (
	"crypto/sha256"
	"testing"
)

func TestCompiledPayloadPinsAndStrictManifest(t *testing.T) {
	payload, err := loadEmbeddedPayload()
	if err != nil || sha256.Sum256(payload.agent) != payload.agentPin || sha256.Sum256(payload.input) != payload.inputPin {
		t.Fatal("compiled payload does not match embedded pins")
	}
	original := embeddedPins
	defer func() { embeddedPins = original }()
	for _, invalid := range [][]byte{nil, []byte(`{}`), append(append([]byte{}, original...), '\n'), []byte(`{"agent":"00","input":"00"}`)} {
		embeddedPins = invalid
		if _, err := loadEmbeddedPayload(); err == nil {
			t.Fatal("noncanonical or invalid embedded pins accepted")
		}
	}
}
