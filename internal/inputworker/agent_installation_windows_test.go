//go:build windows

package inputworker

import (
	"crypto/sha256"
	"testing"
)

func TestAgentLaunchRejectsMissingDigest(t *testing.T) {
	launch, err := OpenAgentLaunch([sha256.Size]byte{})
	if launch != nil || err != ErrAuthority {
		t.Fatal("unbound installed agent accepted")
	}
	var empty *AgentLaunch
	if empty.Path() != "" || empty.Close() != nil {
		t.Fatal("zero launch exposed executable")
	}
}

func TestPortableTestProcessCannotEnableInstalledInput(t *testing.T) {
	digest, err := CurrentAgentServiceDigest()
	if digest != ([sha256.Size]byte{}) || err != ErrAuthority {
		t.Fatal("portable test process enabled privileged input")
	}
}
