package productpanel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

type WorkspaceUpdateLaunch struct {
	Alias, Revision string
	Fingerprint     string
	WaitPID         uint32
}

// Only the verified updater supplies these arguments. Paths and commands are
// never accepted, and the expected build is checked again before reconnecting.
func ParseWorkspaceUpdateLaunch(args []string) (WorkspaceUpdateLaunch, error) {
	fingerprint := ""
	if len(args) == 8 && args[6] == "--peer-fingerprint" {
		fingerprint = args[7]
		if decoded, err := hex.DecodeString(fingerprint); err != nil || len(decoded) != 32 {
			return WorkspaceUpdateLaunch{}, errors.New("invalid update fingerprint")
		}
		args = args[:6]
	}
	if len(args) == 4 && args[0] == "--release-build" && args[2] == "--wait-pid" {
		value, err := ParseWorkspaceUpdateLaunch([]string{"--updated-peer", "release", "--expected-build", args[1], "--wait-pid", args[3]})
		value.Alias = ""
		return value, err
	}
	var v WorkspaceUpdateLaunch
	if (len(args) != 4 && len(args) != 6) || args[0] != "--updated-peer" || args[2] != "--expected-build" {
		return v, errors.New("invalid update arguments")
	}
	v.Alias, v.Revision = args[1], args[3]
	v.Fingerprint = fingerprint
	if v.Alias == "" || len(v.Alias) > 256 || strings.ContainsRune(v.Alias, 0) || len(v.Revision) != 40 {
		return WorkspaceUpdateLaunch{}, errors.New("invalid update identity")
	}
	if _, err := hex.DecodeString(v.Revision); err != nil {
		return WorkspaceUpdateLaunch{}, err
	}
	if len(args) == 6 {
		if args[4] != "--wait-pid" {
			return WorkspaceUpdateLaunch{}, errors.New("invalid update wait")
		}
		pid, err := strconv.ParseUint(args[5], 10, 32)
		if err != nil || pid == 0 {
			return WorkspaceUpdateLaunch{}, errors.New("invalid update pid")
		}
		v.WaitPID = uint32(pid)
	}
	return v, nil
}

type UpdatedWorkspaceClient interface {
	Runtime(context.Context) (localapi.RuntimeInfo, error)
	Trust(context.Context) (localapi.Trust, error)
	Connect(context.Context, string) (localapi.OperationResult, error)
}

func ResumeUpdatedWorkspace(ctx context.Context, c UpdatedWorkspaceClient, v WorkspaceUpdateLaunch) error {
	info, err := c.Runtime(ctx)
	if err != nil || info.Build != v.Revision {
		return errors.New("updated agent build mismatch")
	}
	trust, err := c.Trust(ctx)
	if err != nil {
		return err
	}
	trusted := 0
	for _, p := range trust.Peers {
		if p.Alias == v.Alias && !p.Revoked {
			trusted++
			if v.Fingerprint != "" && p.Fingerprint != v.Fingerprint {
				return errors.New("updated peer identity changed")
			}
		}
	}
	if trusted != 1 {
		return errors.New("updated peer no longer trusted")
	}
	result, err := c.Connect(ctx, v.Alias)
	if err != nil {
		return err
	}
	if result.Status != "COMPLETED" {
		return errors.New("updated connection rejected")
	}
	return nil
}

// A failed replacement may leave the old resident agent fenced in APPLYING.
// Clear only the exact claimed job for the still-trusted launch peer.
func ReportFailedUpdatedWorkspace(ctx context.Context, c UpdateSettingsClient, v WorkspaceUpdateLaunch) bool {
	if c == nil || len(v.Revision) != 40 || v.Alias == "" {
		return false
	}
	result, err := c.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "STATUS"})
	status := result.Status
	if err != nil || result.Code != "ok" || status.State != "APPLYING" || status.Revision != v.Revision || status.Generation != localapi.AgentGeneration || !validTransferID(status.ID) || len(status.Peer) != 64 {
		return false
	}
	trust, err := c.Trust(ctx)
	if err != nil {
		return false
	}
	matched := false
	for _, peer := range trust.Peers {
		if peer.Alias == v.Alias && peer.Fingerprint == status.Peer && !peer.Revoked {
			matched = true
		}
	}
	if !matched {
		return false
	}
	result, err = c.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "APPLY_FAILED", Peer: status.Peer, ID: status.ID})
	return err == nil && result.Code == "apply_failed"
}

// Receiver applies only a READY job whose trust and receive permission remain
// valid. The agent revalidates both when claiming the exact ID. The native
// launcher validates the staged bundle before starting a replacement UI.
type WorkspaceUpdateReceiver struct {
	ctx       context.Context
	open      func() (UpdateSettingsClient, error)
	launch    func(string, string, string, string) error
	mu        sync.Mutex
	attempted string
	retryID   string
	retries   int
	retryAt   time.Time
}

func NewWorkspaceUpdateReceiver(ctx context.Context, open func() (UpdateSettingsClient, error), launch func(string, string, string, string) error) *WorkspaceUpdateReceiver {
	return &WorkspaceUpdateReceiver{ctx: ctx, open: open, launch: launch}
}
func (b *WorkspaceUpdateReceiver) Poll() bool {
	if b == nil || b.ctx == nil || b.open == nil || b.launch == nil || !b.mu.TryLock() {
		return false
	}
	defer b.mu.Unlock()
	ctx, cancel := context.WithTimeout(b.ctx, 15*time.Second)
	defer cancel()
	c, err := b.open()
	if err != nil || c == nil {
		return false
	}
	r, err := c.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "STATUS"})
	s := r.Status
	if err != nil || r.Code != "ok" || s.State != "READY" || !validTransferID(s.ID) || s.ID == b.attempted || len(s.Peer) != 64 || len(s.Revision) != 40 || safeUpdateBuild(s.Revision) == "" {
		return false
	}
	if updatePermission(s.Allowed, s.Peer) != "ALLOWED" {
		return false
	}
	if s.ID == b.retryID && time.Now().Before(b.retryAt) {
		return false
	}
	trust, err := c.Trust(ctx)
	if err != nil {
		return false
	}
	alias := ""
	for _, p := range trust.Peers {
		if p.Fingerprint == s.Peer && !p.Revoked {
			alias = p.Alias
		}
	}
	if alias == "" {
		return false
	}
	before, err := c.Runtime(ctx)
	if err != nil || before.Instance == "" {
		return false
	}
	b.attempted = s.ID
	request := localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "APPLY", Peer: s.Peer, ID: s.ID}
	fail := func() {
		cleanup, done := context.WithTimeout(b.ctx, 5*time.Second)
		defer done()
		request.Action = "APPLY_FAILED"
		_, _ = c.Update(cleanup, request)
	}
	r, err = c.Update(ctx, request)
	if err != nil {
		fail()
		return false
	}
	if r.Code != "apply_ready" {
		// An explicit busy response proves no claim was taken. Retry only this
		// outcome, with a bounded budget; lost responses remain one-shot.
		if r.Code == "apply_busy" {
			if b.retryID != s.ID {
				b.retryID, b.retries = s.ID, 0
			}
			b.retries++
			if b.retries < 3 {
				b.attempted = ""
				b.retryAt = time.Now().Add(5 * time.Second)
			} else {
				fail()
			}
		}
		return false
	}
	after, err := c.Runtime(ctx)
	if err != nil || after.Instance != before.Instance || r.Status.ID != s.ID || r.Status.Peer != s.Peer || r.Status.Revision != s.Revision || r.Status.Generation != s.Generation || r.Alias != alias || ctx.Err() != nil {
		fail()
		return false
	}
	if err := b.launch(r.Directory, s.Revision, alias, s.Peer); err != nil {
		fail()
		return false
	}
	return true
}

// Serialize the handoff with native connect/recovery operations.
func (b *DesktopLifecycle) ApplyReceivedUpdate(receiver *WorkspaceUpdateReceiver) bool {
	if !b.gate.TryLock() {
		return false
	}
	defer b.gate.Unlock()
	if b.updateHandedOff {
		return false
	}
	if receiver.Poll() {
		b.updateHandedOff = true
		b.stopped.Store(true)
		return true
	}
	return false
}

// Public release replacement waits for disconnected input and shares the same
// gate as connect, recovery and peer-update application.
func (b *DesktopLifecycle) ApplyReleaseUpdate(generation int, revision string, launch func() error) bool {
	if launch == nil || !b.gate.TryLock() {
		return false
	}
	defer b.gate.Unlock()
	if b.updateHandedOff {
		return false
	}
	ctx, cancel := b.operation()
	defer cancel()
	return b.applyReleaseUpdate(ctx, generation, revision, launch)
}

// Caller owns gate. The agent-side claim still fences CLI/pairing races.
func (b *DesktopLifecycle) applyReleaseUpdate(ctx context.Context, generation int, revision string, launch func() error) bool {
	view := NewDesktopBridge(ctx, b.open).Refresh()
	if view.Code != "ok" || view.State != "LOCAL" {
		return false
	}
	c, err := b.open()
	if err != nil {
		return false
	}
	api, ok := c.(interface {
		Runtime(context.Context) (localapi.RuntimeInfo, error)
		ReleaseClaim(context.Context, string, string, int, string, bool) (localapi.OperationResult, error)
	})
	if !ok {
		return false
	}
	info, err := api.Runtime(ctx)
	if err != nil {
		return false
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return false
	}
	id := hex.EncodeToString(nonce[:])
	result, err := api.ReleaseClaim(ctx, info.Instance, id, generation, revision, true)
	rollback := func() {
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		_, _ = api.ReleaseClaim(cleanup, info.Instance, id, generation, revision, false)
	}
	if err != nil {
		rollback()
		return false
	}
	if result.Status != "COMPLETED" || result.Code != "release_claimed" {
		return false
	}
	if ctx.Err() != nil {
		rollback()
		return false
	}
	if err := launch(); err != nil {
		rollback()
		return false
	}
	b.updateHandedOff = true
	b.stopped.Store(true)
	return true
}
