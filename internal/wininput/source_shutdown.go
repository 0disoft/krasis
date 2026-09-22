package wininput

import (
	"context"
	"errors"
	"sync"
)

var ErrSourceShutdownPost = errors.New("source shutdown message delivery failed")
var ErrSourceOwnerUnavailable = errors.New("previous input owner did not release before acquisition deadline")

type sourceShutdownKey struct{}
type sourceShutdownEvidence struct {
	mu      sync.Mutex
	failure error
}

func (s *sourceShutdownEvidence) record(err error) {
	if s == nil || err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure == nil {
		s.failure = err
	}
}
func (s *sourceShutdownEvidence) snapshot() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failure
}
func postSourceShutdown(evidence *sourceShutdownEvidence, post func() error) error {
	if err := post(); err != nil {
		failure := errors.Join(ErrSourceShutdownPost, err)
		evidence.record(failure)
		return failure
	}
	return nil
}
func sourceShutdownFrom(ctx context.Context) *sourceShutdownEvidence {
	if ctx == nil {
		return nil
	}
	evidence, _ := ctx.Value(sourceShutdownKey{}).(*sourceShutdownEvidence)
	return evidence
}
