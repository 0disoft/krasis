package residentagent

import (
	"context"
	"sync"

	"github.com/0disoft/krasis/internal/agentlifecycle"
)

type sourceRefreshKey struct{}

type sourceRefreshRefusal byte

const (
	sourceRefreshAccepted sourceRefreshRefusal = iota
	sourceRefreshUnavailable
	sourceRefreshActiveOwner
	sourceRefreshCaptureBusy
)

func (reason sourceRefreshRefusal) code() string {
	switch reason {
	case sourceRefreshActiveOwner:
		return "installed_source_refresh_active_owner"
	case sourceRefreshCaptureBusy:
		return "installed_source_refresh_capture_busy"
	default:
		return "installed_source_refresh_unavailable"
	}
}

// A slot belongs to one connection runner. Native source ownership may register
// only while RunWithBoundary is live; a late owner cannot affect its successor.
type sourceRefreshSlot struct {
	mu         sync.Mutex
	request    func(agentlifecycle.Event) (bool, sourceRefreshRefusal)
	generation uint64
}

func (slot *sourceRefreshSlot) handle(event agentlifecycle.Event) bool {
	accepted, _ := slot.handleObserved(event)
	return accepted
}

func (slot *sourceRefreshSlot) handleObserved(event agentlifecycle.Event) (bool, sourceRefreshRefusal) {
	if slot == nil {
		return false, sourceRefreshUnavailable
	}
	slot.mu.Lock()
	defer slot.mu.Unlock()
	if slot.request == nil {
		return false, sourceRefreshUnavailable
	}
	return slot.request(event)
}

func registerSourceRefresh(ctx context.Context, request func(agentlifecycle.Event) bool) func() {
	if request == nil {
		return registerSourceRefreshObserved(ctx, nil)
	}
	return registerSourceRefreshObserved(ctx, func(event agentlifecycle.Event) (bool, sourceRefreshRefusal) {
		if request(event) {
			return true, sourceRefreshAccepted
		}
		return false, sourceRefreshUnavailable
	})
}

func registerSourceRefreshObserved(ctx context.Context, request func(agentlifecycle.Event) (bool, sourceRefreshRefusal)) func() {
	slot, _ := ctx.Value(sourceRefreshKey{}).(*sourceRefreshSlot)
	if slot == nil {
		return func() {}
	}
	slot.mu.Lock()
	slot.generation++
	generation := slot.generation
	slot.request = request
	slot.mu.Unlock()
	return func() {
		slot.mu.Lock()
		if slot.generation == generation {
			slot.request = nil
		}
		slot.mu.Unlock()
	}
}
