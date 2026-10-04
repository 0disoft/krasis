//go:build windows

package inputworker

import (
	"context"
	"crypto/sha256"
	"io"
	"time"
)

// PrepareConsole authenticates the same service and exact installed agent as a
// lease. The receipt acknowledges only an unbound child, never input authority.
func (service *AgentService) PrepareConsole(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil || service == nil || service.installation == nil || service.digest == ([sha256.Size]byte{}) {
		return ErrAuthority
	}
	_, digest, err := service.installation.digests()
	if err != nil || digest != service.digest {
		return ErrAuthority
	}
	ctx, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
	defer cancel()
	channel, err := connectInstalledService(ctx, digest, service.installation)
	if err != nil {
		return err
	}
	defer channel.Close()
	stop := context.AfterFunc(ctx, func() { _ = channel.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if channel.stream.SetDeadline(deadline) != nil {
		return ErrAuthority
	}
	packet := encodePreparation()
	if writeFrame(channel.stream, packet[:]) != nil {
		return ErrAuthority
	}
	var receipt [8]byte
	if _, err := io.ReadFull(channel.stream, receipt[:]); err != nil || receipt != preparationReceipt || ctx.Err() != nil {
		return ErrAuthority
	}
	return nil
}

var preparationReceipt = [8]byte{'K', 'P', 'A', '1', 1, 0, 0, 0}
