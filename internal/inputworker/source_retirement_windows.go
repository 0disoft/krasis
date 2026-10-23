//go:build windows

package inputworker

import "errors"

func (c *sourceClient) closeResult(result error) error {
	_ = c.channel.Close()
	<-c.done
	// A pending native query can finish while the reader is joining. Inspect
	// cleanup again now: neither its old pending state nor native retirement
	// alone proves that this client's pipe and process-proof handles retired.
	result = closeSourceChannelResult(c.channel, result)
	// Cancellation and the authenticated final packet can arrive together.
	// Join the reader before preserving its terminal native cause; otherwise
	// selecting cancellation first can hide an emergency release and restart
	// capture. An EOF caused by our own Close is not a new native failure.
	if c.retired.Load() && c.err != nil && !errors.Is(result, c.err) {
		return errors.Join(result, c.err)
	}
	return result
}

func closeSourceChannelResult(channel *Channel, result error) error {
	if err := channel.Close(); err != nil {
		return errors.Join(result, ErrRecovery, err)
	}
	return result
}
