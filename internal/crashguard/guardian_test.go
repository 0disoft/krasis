package crashguard

import (
	"bytes"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/0disoft/krasis/internal/control"
)

func guardianCommands(t *testing.T, disarm bool) []byte {
	t.Helper()
	command, err := encodeCommand(operationPreparePress, control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30})
	if err != nil {
		t.Fatal(err)
	}
	data := append([]byte(nil), command[:]...)
	if disarm {
		command, _ = encodeCommand(operationDisarm, control.PressedToken{})
		data = append(data, command[:]...)
	}
	return data
}

func TestGuardianNeverReleasesBeforeInputRetirement(t *testing.T) {
	for _, disarm := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			retired := make(chan struct{})
			started := make(chan struct{})
			done := make(chan struct{})
			release := &recordingRelease{}
			var result Result
			var err error
			var output bytes.Buffer
			go func() {
				result, err = ServeWithRetirement(bytes.NewReader(guardianCommands(t, disarm)), &output, release, func() error {
					close(started)
					<-retired
					return nil
				})
				close(done)
			}()
			<-started
			synctest.Wait()
			if len(release.tokens) != 0 || output.Len() != ackSize {
				t.Fatal("input release/disarm ACK preceded exact child retirement")
			}
			close(retired)
			<-done
			if err != nil || result.Released != 1 || result.Remaining != 0 {
				t.Fatal(result, err)
			}
		})
	}
}

func TestGuardianRetirementFailureRetainsJournalForEveryExit(t *testing.T) {
	proofErr := errors.New("unconfirmed input process")
	for _, suffix := range []string{"", "malformed", "disarm"} {
		t.Run(suffix, func(t *testing.T) {
			data := guardianCommands(t, suffix == "disarm")
			if suffix == "malformed" {
				data = append(data, 0)
			}
			release := &recordingRelease{}
			var output bytes.Buffer
			result, err := ServeWithRetirement(bytes.NewReader(data), &output, release, func() error { return proofErr })
			if !errors.Is(err, proofErr) || !errors.Is(err, ErrRecoveryFailed) || result.Remaining != 1 || result.Attempted != 0 || len(release.tokens) != 0 {
				t.Fatal("unconfirmed owner was treated as released", result, err)
			}
			if suffix == "disarm" {
				if _, err := readAck(bytes.NewReader(output.Bytes()[ackSize:]), operationDisarm); !errors.Is(err, ErrRecoveryFailed) {
					t.Fatal("failed retirement acknowledged disarm", err)
				}
			}
		})
	}
	var calls int
	result, err := ServeWithRetirement(bytes.NewReader(nil), &bytes.Buffer{}, &recordingRelease{}, func() error { calls++; return proofErr })
	if calls != 1 || !errors.Is(err, proofErr) || result.Remaining != 0 {
		t.Fatal("empty journal bypassed input retirement", calls, result, err)
	}
}
