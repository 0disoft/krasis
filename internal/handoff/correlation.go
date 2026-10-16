package handoff

import "context"

// ConfigureExchangeTags installs a connection-local, side-effect-free mapper
// before any session goroutine/attempt starts. It must not perform I/O or make
// authority decisions. No mapper is a supported uncorrelated diagnostic mode.
func ConfigureExchangeTags(ctx context.Context, tagger func(string, string) string) {
	s := getSession(ctx)
	if s == nil || (s.collector.stopped.Load() && !s.collector.rolling) {
		return
	}
	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()
	if s.source == nil && s.receiver == nil && s.exchangeTag == nil {
		s.exchangeTag = tagger
	}
}

func ValidExchangeTag(tag string) bool {
	if len(tag) != 64 {
		return false
	}
	for _, c := range tag {
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
			return false
		}
	}
	return true
}
