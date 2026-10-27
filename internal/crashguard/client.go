package crashguard

import (
	"errors"
	"io"
	"sync"

	"github.com/0disoft/krasis/internal/control"
)

type Client struct {
	mu      sync.Mutex
	writer  io.Writer
	reader  io.Reader
	closed  bool
	failure error
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
	if client.closed || client.failure != nil {
		return client.failure
	}
	client.closed = true
	_, err := client.exchangeLocked(operationDisarm, control.PressedToken{})
	return err
}

func (client *Client) roundTrip(current operation, token control.PressedToken) (status, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.failure != nil {
		return statusRejected, client.failure
	}
	if client.closed {
		return statusRejected, errors.New("crash guard client is closed")
	}
	return client.exchangeLocked(current, token)
}

// Caller holds mu. Once a command may have reached the peer, a failed exchange
// permanently fences this stream: ACKs carry no token or sequence identifier.
func (client *Client) exchangeLocked(current operation, token control.PressedToken) (status, error) {
	payload, err := encodeCommand(current, token)
	if err != nil {
		return statusRejected, err
	}
	n, err := client.writer.Write(payload[:])
	if err == nil && n != len(payload) {
		err = io.ErrShortWrite
	}
	if err != nil {
		client.failure = err
		return statusRejected, err
	}
	result, err := readAck(client.reader, current)
	client.failure = err
	return result, err
}
