package residentagent

import (
	"context"
	"sync"

	"github.com/0disoft/krasis/internal/agentlifecycle"
)

type sourceRefreshKey struct{}

// A slot belongs to one connection runner. Native source ownership may register
// only while RunWithBoundary is live; a late owner cannot affect its successor.
type sourceRefreshSlot struct {
	mu         sync.Mutex
	request    func(agentlifecycle.Event) bool
	generation uint64
}

func (slot *sourceRefreshSlot) handle(event agentlifecycle.Event) bool {
	if slot == nil {
		return false
	}
	slot.mu.Lock()
	defer slot.mu.Unlock()
	return slot.request != nil && slot.request(event)
}

func registerSourceRefresh(ctx context.Context, request func(agentlifecycle.Event) bool) func() {
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
