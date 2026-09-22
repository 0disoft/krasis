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
	protocolVersion  byte = 2
	commandSize           = 12
	ackSize               = 8
	maxPressedInputs      = 256
	recoveryTimeout       = 2 * time.Second
)

var protocolMagic = [4]byte{'K', 'R', 'G', '2'}

type operation byte

const (
	operationPreparePress operation = 1
	operationReleased     operation = 2
	operationDisarm       operation = 3
	operationCancelPress  operation = 4
)

type status byte

const (
	statusOK status = iota
	statusRejected
	statusAlreadyPrepared
)

type ExitReason string

const (
	ExitParentEOF     ExitReason = "PARENT_EOF"
	ExitDisarm        ExitReason = "DISARM"
	ExitProtocolError ExitReason = "PROTOCOL_ERROR"
)

type Result struct {
	Reason    ExitReason
	Attempted int
	Released  int
	Remaining int
}

// Serve owns the fail-safe copy of pressed input state. EOF means that the
// parent disappeared and therefore triggers recovery rather than success.
func Serve(reader io.Reader, writer io.Writer, release application.ReleasePort) error {
	_, err := ServeWithResult(reader, writer, release)
	return err
}

func ServeWithResult(reader io.Reader, writer io.Writer, release application.ReleasePort) (Result, error) {
	if reader == nil || writer == nil || release == nil {
		return Result{}, errors.New("crash guard reader, writer, and release port are required")
	}
	var pressed control.PressedState
	for {
		command, err := readCommand(reader)
		if errors.Is(err, io.EOF) {
			return recoverResult(ExitParentEOF, &pressed, release)
		}
		if err != nil {
			result, recoveryErr := recoverResult(ExitProtocolError, &pressed, release)
			return result, errors.Join(err, recoveryErr)
		}
		switch command.operation {
		case operationPreparePress:
			alreadyPrepared := pressed.Contains(command.token)
			if !pressed.Contains(command.token) && pressed.Count() >= maxPressedInputs {
				_ = writeAck(writer, command.operation, statusRejected)
				result, recoveryErr := recoverResult(ExitProtocolError, &pressed, release)
				return result, errors.Join(
					errors.New("crash guard pressed-input limit exceeded"),
					recoveryErr,
				)
			}
			if _, err := pressed.RecordPress(command.token); err != nil {
				_ = writeAck(writer, command.operation, statusRejected)
				result, recoveryErr := recoverResult(ExitProtocolError, &pressed, release)
				return result, errors.Join(err, recoveryErr)
			}
			ackStatus := statusOK
			if alreadyPrepared {
				ackStatus = statusAlreadyPrepared
			}
			if err := writeAck(writer, command.operation, ackStatus); err != nil {
				result, recoveryErr := recoverResult(ExitProtocolError, &pressed, release)
				return result, errors.Join(err, recoveryErr)
			}
		case operationReleased:
			if _, err := pressed.RecordRelease(command.token); err != nil {
				_ = writeAck(writer, command.operation, statusRejected)
				result, recoveryErr := recoverResult(ExitProtocolError, &pressed, release)
				return result, errors.Join(err, recoveryErr)
			}
			if err := writeAck(writer, command.operation, statusOK); err != nil {
				result, recoveryErr := recoverResult(ExitProtocolError, &pressed, release)
				return result, errors.Join(err, recoveryErr)
			}
		case operationCancelPress:
			if _, err := pressed.RecordRelease(command.token); err != nil {
				_ = writeAck(writer, command.operation, statusRejected)
				result, recoveryErr := recoverResult(ExitProtocolError, &pressed, release)
				return result, errors.Join(err, recoveryErr)
			}
			if err := writeAck(writer, command.operation, statusOK); err != nil {
				result, recoveryErr := recoverResult(ExitProtocolError, &pressed, release)
				return result, errors.Join(err, recoveryErr)
			}
		case operationDisarm:
			result, recoveryErr := recoverResult(ExitDisarm, &pressed, release)
			ackStatus := statusOK
			if recoveryErr != nil {
				ackStatus = statusRejected
			}
			ackErr := writeAck(writer, command.operation, ackStatus)
			return result, errors.Join(recoveryErr, ackErr)
		default:
			result, recoveryErr := recoverResult(ExitProtocolError, &pressed, release)
			return result, errors.Join(
				errors.New("crash guard received an unsupported operation"),
				recoveryErr,
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
	if current != operationPreparePress && current != operationReleased && current != operationCancelPress {
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

func readAck(reader io.Reader, expected operation) (status, error) {
	var payload [ackSize]byte
	if _, err := io.ReadFull(reader, payload[:]); err != nil {
		return statusRejected, err
	}
	if [4]byte(payload[0:4]) != protocolMagic || payload[4] != protocolVersion || operation(payload[5]) != expected || payload[7] != 0 {
		return statusRejected, errors.New("crash guard acknowledgement is invalid")
	}
	result := status(payload[6])
	if expected == operationDisarm && result == statusRejected {
		return result, ErrRecoveryFailed
	}
	if result != statusOK && !(expected == operationPreparePress && result == statusAlreadyPrepared) {
		return result, fmt.Errorf("crash guard rejected operation %d", expected)
	}
	return result, nil
}

func recoverResult(reason ExitReason, pressed *control.PressedState, release application.ReleasePort) (Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), recoveryTimeout)
	defer cancel()
	report, err := application.ReleaseAll(ctx, pressed, release)
	result := Result{Reason: reason, Attempted: report.Attempted, Released: report.Released, Remaining: report.Remaining}
	if err != nil {
		return result, err
	}
	if report.Remaining != 0 {
		return result, fmt.Errorf("crash guard recovery left %d pressed inputs", report.Remaining)
	}
	return result, nil
}
