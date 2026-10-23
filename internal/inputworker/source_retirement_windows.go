//go:build windows

package inputworker

import "errors"

func (c *sourceClient) closeResult(result error) error {
	_ = c.channel.Close()
	<-c.done
	// Cancellation and the authenticated final packet can arrive together.
	// Join the reader before preserving its terminal native cause; otherwise
	// selecting cancellation first can hide an emergency release and restart
	// capture. An EOF caused by our own Close is not a new native failure.
	if c.retired.Load() && c.err != nil && !errors.Is(result, c.err) {
		return errors.Join(result, c.err)
	}
	return result
}
