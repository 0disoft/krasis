//go:build windows && inputinstaller

package main

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"

	"github.com/0disoft/krasis/internal/inputworker"
)

// These files are generated from clean, revision-matched command builds.
// They are compiled into this installer; no adjacent files are loaded at runtime.
//
//go:embed payload/krasis-agent.exe
var embeddedAgent []byte

//go:embed payload/krasis-input.exe
var embeddedInput []byte

//go:embed payload/pins.json
var embeddedPins []byte

func loadEmbeddedPayload() (installerPayload, error) {
	var pins struct {
		Agent string `json:"agent"`
		Input string `json:"input"`
	}
	if json.Unmarshal(embeddedPins, &pins) != nil {
		return installerPayload{}, inputworker.ErrAuthority
	}
	canonical, err := json.Marshal(pins)
	if err != nil || !bytes.Equal(canonical, embeddedPins) {
		return installerPayload{}, inputworker.ErrAuthority
	}
	payload := installerPayload{agent: embeddedAgent, input: embeddedInput}
	for index, text := range []string{pins.Agent, pins.Input} {
		decoded, err := hex.DecodeString(text)
		if err != nil || len(decoded) != sha256.Size || hex.EncodeToString(decoded) != text {
			return installerPayload{}, inputworker.ErrAuthority
		}
		if index == 0 {
			copy(payload.agentPin[:], decoded)
		} else {
			copy(payload.inputPin[:], decoded)
		}
	}
	return payload, nil
}
