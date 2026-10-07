package productpanel

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/appsettings"
)

// Native startup only. Retrying a read never retries a connection command or
// rewrites settings. The agent arbitrates a manual stop racing this read.
func ResumeAtLogin(ctx context.Context, value BridgeSnapshot, load func() (appsettings.Resume, error), resume func(string, string) BridgeSnapshot) BridgeSnapshot {
	if value.State != "LOCAL" || value.Code != "ok" {
		return value
	}
	var peer appsettings.Resume
	for attempt := 0; attempt < 3; attempt++ {
		if ctx.Err() != nil {
			value.Code = "cancelled"
			return value
		}
		var err error
		peer, err = load()
		if err == nil {
			break
		}
		if errors.Is(err, appsettings.ErrInvalidResume) {
			value.Code = "startup_resume_settings_invalid"
			return value
		}
		if attempt == 2 {
			value.Code = "startup_resume_settings_unavailable"
			return value
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			value.Code = "cancelled"
			return value
		case <-timer.C:
		}
	}
	if peer.ManualStop || peer.Alias == "" {
		return value
	}
	matches := 0
	for _, p := range value.Peers {
		if p.Alias == peer.Alias {
			if p.Fingerprint != peer.Fingerprint {
				value.Code = "stale_peer"
				return value
			}
			matches++
		}
	}
	if matches != 1 {
		value.Code = "stale_peer"
		return value
	}
	if ctx.Err() != nil {
		value.Code = "cancelled"
		return value
	}
	return resume(peer.Alias, peer.Fingerprint)
}
