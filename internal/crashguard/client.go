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

func (client *Client) PreparePress(token control.PressedToken) error {
	return client.roundTrip(operationPreparePress, token)
}

func (client *Client) Released(token control.PressedToken) error {
	return client.roundTrip(operationReleased, token)
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
	return readAck(client.reader, operationDisarm)
}

func (client *Client) roundTrip(current operation, token control.PressedToken) error {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closed {
		return errors.New("crash guard client is closed")
	}
	payload, err := encodeCommand(current, token)
	if err != nil {
		return err
	}
	if _, err := client.writer.Write(payload[:]); err != nil {
		return err
	}
	return readAck(client.reader, current)
}
