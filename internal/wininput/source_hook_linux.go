package wininput

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/x11desktop"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"sync"
	"time"
)

const linuxPollInterval = 4 * time.Millisecond
const linuxEventBatch = 256

type SourceHook struct {
	mu                     sync.Mutex
	boundaryGuard          func() error
	desktop                *x11desktop.Desktop
	session                *SourceSession
	root                   context.Context
	active                 bool
	lastX, lastY           int32
	returnHidden           bool
	presentationGeneration uint64
	nextHiddenInspection   time.Time
	report                 SourcePresentationReporter
}

func NewSourceHook(...HookTimingObserver) *SourceHook                      { return &SourceHook{} }
func (h *SourceHook) SetPresentationReporter(r SourcePresentationReporter) { h.report = r }
func (*SourceHook) SetAdmissionReporter(func(string))                      {}
func (h *SourceHook) SourceCapture() SourceActivation                      { return h }
func (h *SourceHook) Active() bool                                         { h.mu.Lock(); defer h.mu.Unlock(); return h.active }
func (h *SourceHook) Activate() error                                      { return h.ActivateProfile(context.Background()) }
func (h *SourceHook) ActivateProfile(ctx context.Context) error {
	if h.boundaryGuard != nil {
		if err := h.boundaryGuard(); err != nil {
			return err
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.desktop == nil || h.active {
		return ErrSourceSession
	}
	if err := h.root.Err(); err != nil {
		return err
	}
	if err := h.desktop.Grab(ctx); err != nil {
		return err
	}
	held, err := h.desktop.AnyPressed(ctx)
	if err != nil || held {
		cleanup, cancel := context.WithTimeout(context.Background(), SourceCleanupTimeout)
		defer cancel()
		return errors.Join(errors.New("physical input changed during X11 grab"), err, h.desktop.Ungrab(cleanup))
	}
	h.active = true
	h.presentationGeneration++
	h.returnHidden = false
	h.lastX, h.lastY = h.desktop.Center()
	return nil
}
func (h *SourceHook) Deactivate() { h.mu.Lock(); defer h.mu.Unlock(); h.deactivateLocked() }
func (h *SourceHook) deactivateLocked() {
	if h.desktop == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), SourceCleanupTimeout)
	defer cancel()
	if err := h.desktop.Ungrab(ctx); err != nil && h.session != nil {
		if h.root.Err() != nil {
			_ = h.desktop.Close()
		}
		if !linuxSourceCancellationOnly(err, h.root.Err()) {
			h.session.Abort(err)
		}
	}
	h.active = false
	for i := 0; i < linuxEventBatch; i++ {
		e, err := h.desktop.Poll()
		if err != nil || e == nil {
			break
		}
	}
	if x, y, err := h.desktop.Position(ctx); err == nil {
		h.lastX, h.lastY = x, y
	}
}
func (h *SourceHook) Run(ctx context.Context, s *SourceSession, ready func()) error {
	return h.RunWithBoundary(ctx, s, nil, ready)
}
func (h *SourceHook) RunWithBoundary(ctx context.Context, s *SourceSession, b BoundaryObserver, ready func()) (result error) {
	if ctx == nil || s == nil {
		return ErrSourceSession
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	d, err := x11desktop.Open(ctx)
	if err != nil {
		return err
	}
	defer d.Close()
	x, y, err := d.Position(ctx)
	if err != nil {
		return err
	}
	h.mu.Lock()
	if h.desktop != nil {
		h.mu.Unlock()
		return errors.New("source hook already running")
	}
	h.desktop = d
	h.session = s
	h.root = ctx
	h.lastX, h.lastY = x, y
	h.mu.Unlock()
	s.physicalMu.Lock()
	s.physicalCheck = func() bool { held, err := d.AnyPressed(ctx); return err != nil || held }
	s.advancePhysicalRevision()
	s.physicalMu.Unlock()
	defer func() {
		h.mu.Lock()
		h.deactivateLocked()
		cleanup, cancel := context.WithTimeout(context.Background(), SourceCleanupTimeout)
		result = errors.Join(result, d.ShowCursor(cleanup, true))
		cancel()
		// X11 releases this client's grabs and cursor visibility on close.
		// Cancellation may have already closed an in-flight query's socket.
		// Confirm the owner is closed before treating that closure as expected.
		_ = d.Close()
		if linuxSourceCancellationOnly(result, ctx.Err()) {
			result = ctx.Err()
		}
		h.desktop = nil
		h.session = nil
		h.mu.Unlock()
		s.physicalMu.Lock()
		s.physicalCheck = nil
		s.advancePhysicalRevision()
		s.physicalMu.Unlock()
	}()
	if ready != nil {
		ready()
	}
	ticker := time.NewTicker(linuxPollInterval)
	defer ticker.Stop()
	geometryTick := time.Now()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			h.mu.Lock()
			err = h.tickLocked(ctx, s, b)
			h.mu.Unlock()
			if err != nil {
				return err
			}
			if time.Since(geometryTick) >= time.Second {
				g, e := d.Geometry(ctx)
				if e != nil {
					return e
				}
				if g != d.Dimensions() {
					return ErrDisplayGeometryChanged
				}
				geometryTick = time.Now()
			}
		}
	}
}

func (h *SourceHook) tickLocked(ctx context.Context, s *SourceSession, b BoundaryObserver) error {
	if h.active && !s.Active() {
		h.deactivateLocked()
	}
	if !h.active {
		if h.returnHidden && !time.Now().Before(h.nextHiddenInspection) {
			h.nextHiddenInspection = time.Now().Add(25 * time.Millisecond)
			held, err := h.desktop.AnyPressed(ctx)
			if err != nil {
				return err
			}
			if held {
				if err = h.desktop.ShowCursor(ctx, true); err != nil {
					return err
				}
				h.presentationGeneration++
				h.returnHidden = false
			}
		}
		x, y, err := h.desktop.Position(ctx)
		if err != nil {
			return err
		}
		dx, dy := x-h.lastX, y-h.lastY
		h.lastX, h.lastY = x, y
		if dx != 0 || dy != 0 {
			h.presentationGeneration++
			if h.returnHidden {
				if err = h.desktop.ShowCursor(ctx, true); err != nil {
					return err
				}
				h.returnHidden = false
			}
			routePhysicalAction(s, b, control.InputAction{Kind: control.InputPointerMove, DeltaX: dx, DeltaY: dy}, x)
		}
		return nil
	}
	for i := 0; i < linuxEventBatch; i++ {
		e, err := h.desktop.Poll()
		if err != nil {
			return err
		}
		if e == nil {
			break
		}
		a, emit, x, err := h.mapEvent(e)
		if err != nil {
			return err
		}
		if !emit {
			continue
		}
		if a.Kind == control.InputKeyDown && a.Code == 88 && linuxEmergencyReturn(s) {
			s.RequestLocalReturn()
			h.deactivateLocked()
			return nil
		}
		if !routePhysicalAction(s, b, a, x) && s.Active() {
			return errors.New("X11 capture action rejected")
		}
	}
	return nil
}
func linuxEmergencyReturn(s *SourceSession) bool {
	s.physicalMu.Lock()
	defer s.physicalMu.Unlock()
	has := func(code uint32) bool { _, ok := s.physicalHeld[physicalPress{code: code}]; return ok }
	return (has(29) || has(0x1001d)) && (has(56) || has(0x10038)) && (has(42) || has(54))
}

func (h *SourceHook) mapEvent(e xgb.Event) (control.InputAction, bool, int32, error) {
	var a control.InputAction
	switch v := e.(type) {
	case xproto.MotionNotifyEvent:
		x, y := int32(v.RootX), int32(v.RootY)
		dx, dy := x-h.lastX, y-h.lastY
		cx, cy := h.desktop.Center()
		h.lastX, h.lastY = x, y
		if x == cx && y == cy || dx == 0 && dy == 0 {
			return a, false, 0, nil
		}
		if err := h.desktop.SetPosition(h.root, cx, cy); err != nil {
			return a, false, 0, err
		}
		a = control.InputAction{Kind: control.InputPointerMove, DeltaX: dx, DeltaY: dy}
	case xproto.KeyPressEvent:
		code, ok := h.desktop.Scan(byte(v.Detail))
		if !ok {
			return a, false, 0, errors.New("unsupported XKB physical key")
		}
		a = control.InputAction{Kind: control.InputKeyDown, Code: code}
	case xproto.KeyReleaseEvent:
		code, ok := h.desktop.Scan(byte(v.Detail))
		if !ok {
			return a, false, 0, errors.New("unsupported XKB physical key")
		}
		a = control.InputAction{Kind: control.InputKeyUp, Code: code}
	case xproto.ButtonPressEvent:
		return linuxButtonAction(byte(v.Detail), true)
	case xproto.ButtonReleaseEvent:
		return linuxButtonAction(byte(v.Detail), false)
	case xproto.MappingNotifyEvent:
		return a, false, 0, errors.New("keyboard mapping changed during capture")
	default:
		return a, false, 0, nil
	}
	return a, true, h.lastX, a.Validate()
}
func linuxButtonAction(button byte, down bool) (control.InputAction, bool, int32, error) {
	a := control.InputAction{Kind: control.InputButtonUp}
	if down {
		a.Kind = control.InputButtonDown
	}
	switch button {
	case 1:
		a.Code = MouseButtonLeft
	case 2:
		a.Code = MouseButtonMiddle
	case 3:
		a.Code = MouseButtonRight
	case 8:
		a.Code = MouseButtonX1
	case 9:
		a.Code = MouseButtonX2
	case 4, 5, 6, 7:
		if !down {
			return a, false, 0, nil
		}
		a.Kind = control.InputWheel
		switch button {
		case 4:
			a.DeltaY = 120
		case 5:
			a.DeltaY = -120
		case 6:
			a.DeltaX = -120
		case 7:
			a.DeltaX = 120
		}
	default:
		return a, false, 0, errors.New("unsupported X11 pointer button")
	}
	return a, true, 0, nil
}
func (h *SourceHook) PrepareReturnCursor() func() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.desktop == nil || h.active {
		return nil
	}
	d := h.desktop
	ctx, cancel := context.WithTimeout(context.Background(), SourceCleanupTimeout)
	defer cancel()
	x, y, err := d.Position(ctx)
	if err != nil {
		h.session.Abort(err)
		return nil
	}
	h.presentationGeneration++
	generation := h.presentationGeneration
	// Do not change the cursor before the lease return commits. Failed/stale
	// returns must leave local presentation alone; a later entry wins this token.
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.desktop != d || h.active || generation != h.presentationGeneration {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), SourceCleanupTimeout)
		defer cancel()
		px, py, err := d.Position(ctx)
		if err != nil {
			h.session.Abort(err)
			return
		}
		if px != x || py != y {
			return
		}
		if err = d.ShowCursor(ctx, false); err != nil {
			h.session.Abort(err)
			return
		}
		h.returnHidden = true
	}
}
func (h *SourceHook) RestoreReturnCursor() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.presentationGeneration++
	if h.desktop != nil && !h.active {
		ctx, cancel := context.WithTimeout(context.Background(), SourceCleanupTimeout)
		defer cancel()
		if err := h.desktop.ShowCursor(ctx, true); err != nil {
			h.session.Abort(err)
		}
	}
	h.returnHidden = false
}
