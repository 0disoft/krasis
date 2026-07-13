package crashguard

import (
	"errors"
	"io"
	"sync"

	"github.com/0disoft/krasis/internal/control"
)

type Client struct {
	mu     sync.Mutex
	writer io.Writer
	reader io.Reader
	closed bool
}

func NewClient(writer io.Writer, reader io.Reader) (*Client, error) {
	if writer == nil || reader == nil {
		return nil, errors.New("crash guard client reader and writer are required")
	}
	return &Client{writer: writer, reader: reader}, nil
}

func (client *Client) PreparePress(token control.PressedToken) (bool, error) {
	result, err := client.roundTrip(operationPreparePress, token)
	return result == statusOK, err
}

func (client *Client) Released(token control.PressedToken) error {
	_, err := client.roundTrip(operationReleased, token)
	return err
}

func (client *Client) CancelPreparedPress(token control.PressedToken) error {
	_, err := client.roundTrip(operationCancelPress, token)
	return err
}

func (client *Client) Disarm() error {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closed {
		return nil
	}
	client.closed = true
	payload, err := encodeCommand(operationDisarm, control.PressedToken{})
	if err != nil {
		return err
	}
	if _, err := client.writer.Write(payload[:]); err != nil {
		return err
	}
	_, err = readAck(client.reader, operationDisarm)
	return err
}

func (client *Client) roundTrip(current operation, token control.PressedToken) (status, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closed {
		return statusRejected, errors.New("crash guard client is closed")
	}
	payload, err := encodeCommand(current, token)
	if err != nil {
		return statusRejected, err
	}
	if _, err := client.writer.Write(payload[:]); err != nil {
		return statusRejected, err
	}
	return readAck(client.reader, current)
}
