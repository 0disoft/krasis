package wininput

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/x11desktop"
	"sync"
)

// The guard child opens its recovery connection lazily. The resident opens
// before admission and closes its native owner after the guard has joined.
type linuxInjection struct {
	mu      sync.Mutex
	desktop *x11desktop.Desktop
	closed  bool
}

func (p *linuxInjection) get(ctx context.Context) (*x11desktop.Desktop, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, errors.New("Linux input owner closed")
	}
	if p.desktop == nil {
		var err error
		p.desktop, err = x11desktop.Open(ctx)
		if err != nil {
			return nil, err
		}
	}
	return p.desktop, nil
}
func (p *linuxInjection) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	if p.desktop != nil {
		return p.desktop.Close()
	}
	return nil
}
func (p *linuxInjection) Press(c context.Context, t control.PressedToken) error {
	return p.discrete(c, t, true)
}
func (p *linuxInjection) Release(c context.Context, t control.PressedToken) error {
	return p.discrete(c, t, false)
}
func (p *linuxInjection) discrete(ctx context.Context, t control.PressedToken, down bool) error {
	if err := t.Validate(); err != nil {
		return err
	}
	d, err := p.get(ctx)
	if err != nil {
		return err
	}
	switch t.Kind {
	case control.PressedKeyboardKey:
		return d.Key(ctx, t.Code, down)
	case control.PressedMouseButton:
		return d.Button(ctx, t.Code, down)
	}
	return ErrUnsupportedInput
}
func (p *linuxInjection) Move(ctx context.Context, x, y int32) error {
	_, _, err := p.MoveWithPosition(ctx, x, y)
	return err
}
func (p *linuxInjection) MoveWithPosition(ctx context.Context, x, y int32) (int32, int32, error) {
	d, err := p.get(ctx)
	if err != nil {
		return 0, 0, err
	}
	return d.Move(ctx, x, y)
}
func (p *linuxInjection) Wheel(ctx context.Context, x, y int32) error {
	d, err := p.get(ctx)
	if err != nil {
		return err
	}
	return d.Wheel(ctx, x, y)
}
func (p *linuxInjection) CursorPosition(ctx context.Context) (int32, int32, error) {
	d, err := p.get(ctx)
	if err != nil {
		return 0, 0, err
	}
	return d.Position(ctx)
}
func (p *linuxInjection) SetCursorPosition(ctx context.Context, x, y int32) error {
	d, err := p.get(ctx)
	if err != nil {
		return err
	}
	return d.SetPosition(ctx, x, y)
}
func (p *linuxInjection) displayGeometry(ctx context.Context) (DisplayGeometry, error) {
	d, err := p.get(ctx)
	if err != nil {
		return DisplayGeometry{}, err
	}
	g := d.Dimensions()
	return DisplayGeometry{Count: g.Count, Width: g.Width, Height: g.Height}, nil
}

func NewInputAdapter() *InputAdapter {
	a := newInputAdapter()
	a.injection = &linuxInjection{}
	return a
}
func OpenLinuxInput(ctx context.Context, distance, width int32) (*InputAdapter, func() error, error) {
	p := &linuxInjection{}
	if _, err := p.get(ctx); err != nil {
		return nil, nil, err
	}
	a, err := NewInputAdapterWithInjection(distance, width, p)
	if err != nil {
		_ = p.Close()
		return nil, nil, err
	}
	return a, p.Close, nil
}
func (a *InputAdapter) cursorPort() (application.CursorPositionPort, error) {
	if a == nil {
		return nil, ErrInputInjection
	}
	p, ok := a.injection.(application.CursorPositionPort)
	if !ok {
		return nil, ErrInputInjection
	}
	return p, nil
}
func (a *InputAdapter) PrepareEntry(ctx context.Context, edge control.ScreenEdge) error {
	if ctx == nil {
		return ErrInputInjection
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s, err := a.cursorEntryState()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending {
		return ErrCursorEntryState
	}
	p, err := a.cursorPort()
	if err != nil {
		return err
	}
	x, y, err := p.CursorPosition(ctx)
	if err != nil {
		return err
	}
	g, err := a.geometry(ctx)
	if err != nil {
		return err
	}
	if !g.Valid() {
		return ErrDisplayGeometryChanged
	}
	tx, ty, err := cursorEntryPoint(edge, g.Left, g.Top, g.Width, g.Height)
	if err != nil {
		return err
	}
	s.pending = true
	s.previousX, s.previousY = x, y
	if err = p.SetCursorPosition(ctx, tx, ty); err != nil {
		return err
	}
	x, y, err = p.CursorPosition(ctx)
	if err != nil {
		return err
	}
	s.enteredKnown = true
	s.enteredX, s.enteredY = x, y
	if x != tx || y != ty {
		return application.ErrCursorEntryMismatch
	}
	return nil
}
func (a *InputAdapter) RollbackEntry(ctx context.Context) (application.CursorRollbackOutcome, error) {
	s, err := a.cursorEntryState()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.pending {
		return application.CursorRollbackNotRestored, nil
	}
	defer s.clear()
	if ctx == nil {
		return "", ErrCursorEntryState
	}
	if err := ctx.Err(); err != nil {
		return "", errors.Join(ErrCursorEntryState, err)
	}
	if !s.enteredKnown {
		return "", ErrCursorEntryState
	}
	p, err := a.cursorPort()
	if err != nil {
		return "", err
	}
	x, y, err := p.CursorPosition(ctx)
	if err != nil {
		return "", err
	}
	if x != s.enteredX || y != s.enteredY {
		return application.CursorRollbackNotRestored, nil
	}
	if err = p.SetCursorPosition(ctx, s.previousX, s.previousY); err != nil {
		return "", err
	}
	x, y, err = p.CursorPosition(ctx)
	if err != nil {
		return "", err
	}
	if x != s.previousX || y != s.previousY {
		return "", ErrCursorEntryState
	}
	return application.CursorRollbackRestored, nil
}
func (a *InputAdapter) Press(ctx context.Context, t control.PressedToken) error {
	if a == nil || a.injection == nil {
		return ErrInputInjection
	}
	if err := a.injection.Press(ctx, t); err != nil {
		return err
	}
	return a.recordReturnInput(t, true)
}
func (a *InputAdapter) Release(ctx context.Context, t control.PressedToken) error {
	if a == nil || a.injection == nil {
		return ErrInputInjection
	}
	if err := a.injection.Release(ctx, t); err != nil {
		return err
	}
	return a.recordReturnInput(t, false)
}
func (a *InputAdapter) Move(ctx context.Context, x, y int32) error {
	if a == nil || a.injection == nil {
		return ErrInputInjection
	}
	p, ok := a.injection.(application.PositionedMovePort)
	if !ok {
		return ErrInputInjection
	}
	px, _, err := p.MoveWithPosition(ctx, x, y)
	if err != nil {
		return err
	}
	g, err := a.geometry(ctx)
	if err != nil {
		return err
	}
	if !g.Valid() {
		return ErrDisplayGeometryChanged
	}
	return a.observeReturnPosition(px, g.Left, g.Width)
}

// Reuse the pinned native owner's session geometry on the motion hot path.
func (a *InputAdapter) geometry(ctx context.Context) (DisplayGeometry, error) {
	if p, ok := a.injection.(interface {
		displayGeometry(context.Context) (DisplayGeometry, error)
	}); ok {
		return p.displayGeometry(ctx)
	}
	return CurrentDisplayGeometry(), nil
}

func (a *InputAdapter) Wheel(ctx context.Context, x, y int32) error {
	if a == nil || a.injection == nil {
		return ErrInputInjection
	}
	return a.injection.Wheel(ctx, x, y)
}
