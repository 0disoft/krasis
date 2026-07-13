package crashguard

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/0disoft/krasis/internal/control"
)

type recordingRelease struct {
	mu     sync.Mutex
	tokens []control.PressedToken
	fail   map[control.PressedToken]error
}

func (release *recordingRelease) Release(_ context.Context, token control.PressedToken) error {
	release.mu.Lock()
	defer release.mu.Unlock()
	release.tokens = append(release.tokens, token)
	return release.fail[token]
}

func TestServeRecoversPreparedInputsOnParentEOF(t *testing.T) {
	serverInput, commands := io.Pipe()
	acknowledgements, serverOutput := io.Pipe()
	release := &recordingRelease{fail: map[control.PressedToken]error{}}
	done := make(chan error, 1)
	go func() { done <- Serve(serverInput, serverOutput, release) }()
	client, err := NewClient(commands, acknowledgements)
	if err != nil {
		t.Fatal(err)
	}
	key := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}
	button := control.PressedToken{Kind: control.PressedMouseButton, Code: 1}
	if prepared, err := client.PreparePress(key); err != nil || !prepared {
		t.Fatal(err)
	}
	if prepared, err := client.PreparePress(button); err != nil || !prepared {
		t.Fatal(err)
	}
	if err := commands.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	want := []control.PressedToken{button, key}
	if len(release.tokens) != len(want) || release.tokens[0] != want[0] || release.tokens[1] != want[1] {
		t.Fatalf("released tokens = %+v, want %+v", release.tokens, want)
	}
}

func TestServeDoesNotRecoverReleasedInput(t *testing.T) {
	serverInput, commands := io.Pipe()
	acknowledgements, serverOutput := io.Pipe()
	release := &recordingRelease{fail: map[control.PressedToken]error{}}
	done := make(chan error, 1)
	go func() { done <- Serve(serverInput, serverOutput, release) }()
	client, _ := NewClient(commands, acknowledgements)
	token := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}
	if prepared, err := client.PreparePress(token); err != nil || !prepared {
		t.Fatal(err)
	}
	if err := client.Released(token); err != nil {
		t.Fatal(err)
	}
	if err := client.Disarm(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	if len(release.tokens) != 0 {
		t.Fatalf("released tokens = %+v", release.tokens)
	}
}

func TestServeCancelsOnlyNewPreparedInput(t *testing.T) {
	serverInput, commands := io.Pipe()
	acknowledgements, serverOutput := io.Pipe()
	release := &recordingRelease{fail: map[control.PressedToken]error{}}
	done := make(chan error, 1)
	go func() { done <- Serve(serverInput, serverOutput, release) }()
	client, _ := NewClient(commands, acknowledgements)
	token := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}

	prepared, err := client.PreparePress(token)
	if err != nil || !prepared {
		t.Fatalf("first PreparePress() = (%t, %v)", prepared, err)
	}
	prepared, err = client.PreparePress(token)
	if err != nil || prepared {
		t.Fatalf("duplicate PreparePress() = (%t, %v)", prepared, err)
	}
	if err := client.CancelPreparedPress(token); err != nil {
		t.Fatal(err)
	}
	if err := client.Disarm(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	if len(release.tokens) != 0 {
		t.Fatalf("canceled press was injected as release: %+v", release.tokens)
	}
}

func TestServeReportsRecoveryFailure(t *testing.T) {
	token := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}
	payload, err := encodeCommand(operationPreparePress, token)
	if err != nil {
		t.Fatal(err)
	}
	release := &recordingRelease{fail: map[control.PressedToken]error{token: errors.New("injection failed")}}
	var output bytes.Buffer
	err = Serve(bytes.NewReader(payload[:]), &output, release)
	if err == nil || !errors.Is(err, release.fail[token]) {
		t.Fatalf("Serve() error = %v", err)
	}
}

func TestReadCommandRejectsMalformedAndTrailingPartialFrames(t *testing.T) {
	malformed := make([]byte, commandSize)
	if _, err := readCommand(bytes.NewReader(malformed)); err == nil {
		t.Fatal("readCommand accepted malformed header")
	}
	if _, err := readCommand(bytes.NewReader([]byte("KRG1"))); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("partial frame error = %v", err)
	}
}

func TestServeRecoversPreparedInputAfterMalformedCommand(t *testing.T) {
	token := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}
	prepared, err := encodeCommand(operationPreparePress, token)
	if err != nil {
		t.Fatal(err)
	}
	input := append(prepared[:], make([]byte, commandSize)...)
	release := &recordingRelease{fail: map[control.PressedToken]error{}}
	var output bytes.Buffer
	if err := Serve(bytes.NewReader(input), &output, release); err == nil {
		t.Fatal("Serve accepted malformed command")
	}
	if len(release.tokens) != 1 || release.tokens[0] != token {
		t.Fatalf("released tokens = %+v", release.tokens)
	}
}
