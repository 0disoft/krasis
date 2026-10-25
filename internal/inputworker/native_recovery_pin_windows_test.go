//go:build windows

package inputworker

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestRecoveryServicePinRequiresOneFinalLiveProof(t *testing.T) {
	for _, mode := range []string{"valid", "service_restarted", "proof_closed", "wrong_kind"} {
		t.Run(mode, func(t *testing.T) {
			server, client := nativePair(t)
			defer server.Close()
			defer client.Close()
			checks := 0
			proof := &Authority{kind: authorityService, checkInput: func() error {
				checks++
				if mode == "service_restarted" {
					return ErrAuthority
				}
				return nil
			}, checkRecovery: func() error { return nil }, closeHandles: func() error { return nil }}
			if mode == "proof_closed" {
				_ = proof.Close()
			} else if mode == "wrong_kind" {
				proof.kind = authorityAgent
			}
			handle, err := pinNativeRecoveryService(client.handle, proof)
			if mode != "valid" {
				if handle != 0 || err != ErrAuthority {
					t.Fatal("invalid service published a recovery pin", handle, err)
				}
				return
			}
			defer windows.CloseHandle(handle)
			if err != nil || handle == 0 || checks != 1 || !nativeParentAlive(handle) {
				t.Fatal("exact process pin lacks one final service check", handle, err, checks)
			}
		})
	}
}
