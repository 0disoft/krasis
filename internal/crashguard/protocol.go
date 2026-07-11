package crashguard

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

const (
	protocolVersion  byte = 1
	commandSize           = 12
	ackSize               = 8
	maxPressedInputs      = 256
	recoveryTimeout       = 2 * time.Second
)

var protocolMagic = [4]byte{'K', 'R', 'G', '1'}

type operation byte

const (
	operationPreparePress operation = 1
	operationReleased     operation = 2
	operationDisarm       operation = 3
)

type status byte

const (
	statusOK status = iota
	statusRejected
)

// Serve owns the fail-safe copy of pressed input state. EOF means that the
// parent disappeared and therefore triggers recovery rather than success.
func Serve(reader io.Reader, writer io.Writer, release application.ReleasePort) error {
	if reader == nil || writer == nil || release == nil {
		return errors.New("crash guard reader, writer, and release port are required")
	}
	var pressed control.PressedState
	for {
		command, err := readCommand(reader)
		if errors.Is(err, io.EOF) {
			return recoverPressed(&pressed, release)
		}
		if err != nil {
			return errors.Join(err, recoverPressed(&pressed, release))
		}
		switch command.operation {
		case operationPreparePress:
			if !pressed.Contains(command.token) && pressed.Count() >= maxPressedInputs {
				_ = writeAck(writer, command.operation, statusRejected)
				return errors.Join(
					errors.New("crash guard pressed-input limit exceeded"),
					recoverPressed(&pressed, release),
				)
			}
			if _, err := pressed.RecordPress(command.token); err != nil {
				_ = writeAck(writer, command.operation, statusRejected)
				return errors.Join(err, recoverPressed(&pressed, release))
			}
			if err := writeAck(writer, command.operation, statusOK); err != nil {
				return errors.Join(err, recoverPressed(&pressed, release))
			}
		case operationReleased:
			if _, err := pressed.RecordRelease(command.token); err != nil {
				_ = writeAck(writer, command.operation, statusRejected)
				return errors.Join(err, recoverPressed(&pressed, release))
			}
			if err := writeAck(writer, command.operation, statusOK); err != nil {
				return errors.Join(err, recoverPressed(&pressed, release))
			}
		case operationDisarm:
			recoveryErr := recoverPressed(&pressed, release)
			ackStatus := statusOK
			if recoveryErr != nil {
				ackStatus = statusRejected
			}
			ackErr := writeAck(writer, command.operation, ackStatus)
			return errors.Join(recoveryErr, ackErr)
		default:
			return errors.Join(
				errors.New("crash guard received an unsupported operation"),
				recoverPressed(&pressed, release),
			)
		}
	}
}

type wireCommand struct {
	operation operation
	token     control.PressedToken
}

func readCommand(reader io.Reader) (wireCommand, error) {
	var payload [commandSize]byte
	if _, err := io.ReadFull(reader, payload[:]); err != nil {
		return wireCommand{}, err
	}
	if [4]byte(payload[0:4]) != protocolMagic || payload[4] != protocolVersion || payload[7] != 0 {
		return wireCommand{}, errors.New("crash guard command header is invalid")
	}
	command := wireCommand{operation: operation(payload[5])}
	if command.operation == operationDisarm {
		if payload[6] != 0 || binary.BigEndian.Uint32(payload[8:12]) != 0 {
			return wireCommand{}, errors.New("crash guard disarm command contains input data")
		}
		return command, nil
	}
	switch payload[6] {
	case 1:
		command.token.Kind = control.PressedKeyboardKey
	case 2:
		command.token.Kind = control.PressedMouseButton
	default:
		return wireCommand{}, errors.New("crash guard command has an invalid input kind")
	}
	command.token.Code = binary.BigEndian.Uint32(payload[8:12])
	if err := command.token.Validate(); err != nil {
		return wireCommand{}, err
	}
	return command, nil
}

func encodeCommand(current operation, token control.PressedToken) ([commandSize]byte, error) {
	var payload [commandSize]byte
	copy(payload[0:4], protocolMagic[:])
	payload[4] = protocolVersion
	payload[5] = byte(current)
	if current == operationDisarm {
		return payload, nil
	}
	if current != operationPreparePress && current != operationReleased {
		return payload, errors.New("unsupported crash guard operation")
	}
	if err := token.Validate(); err != nil {
		return payload, err
	}
	switch token.Kind {
	case control.PressedKeyboardKey:
		payload[6] = 1
	case control.PressedMouseButton:
		payload[6] = 2
	}
	binary.BigEndian.PutUint32(payload[8:12], token.Code)
	return payload, nil
}

func writeAck(writer io.Writer, current operation, result status) error {
	var payload [ackSize]byte
	copy(payload[0:4], protocolMagic[:])
	payload[4] = protocolVersion
	payload[5] = byte(current)
	payload[6] = byte(result)
	_, err := writer.Write(payload[:])
	return err
}

func readAck(reader io.Reader, expected operation) error {
	var payload [ackSize]byte
	if _, err := io.ReadFull(reader, payload[:]); err != nil {
		return err
	}
	if [4]byte(payload[0:4]) != protocolMagic || payload[4] != protocolVersion || operation(payload[5]) != expected || payload[7] != 0 {
		return errors.New("crash guard acknowledgement is invalid")
	}
	if status(payload[6]) != statusOK {
		return fmt.Errorf("crash guard rejected operation %d", expected)
	}
	return nil
}

func recoverPressed(pressed *control.PressedState, release application.ReleasePort) error {
	ctx, cancel := context.WithTimeout(context.Background(), recoveryTimeout)
	defer cancel()
	report, err := application.ReleaseAll(ctx, pressed, release)
	if err != nil {
		return err
	}
	if report.Remaining != 0 {
		return fmt.Errorf("crash guard recovery left %d pressed inputs", report.Remaining)
	}
	return nil
}
