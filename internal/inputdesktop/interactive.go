package inputdesktop

import "errors"

// Only a service-authenticated, privileged input owner supplies these callbacks.
// A switch retires the old binding before acquiring another fixed desktop. No
// input operation is repeated, and failed native cleanup permanently fences it.
type interactiveBinding struct {
	current   binding
	open      func() (binding, error)
	authorize func() error
	fenced    bool
}

func (b *interactiveBinding) active() error {
	if b.fenced {
		return ErrCleanup
	}
	if err := b.authorize(); err != nil {
		return err
	}
	if b.current != nil {
		if err := b.current.active(); err == nil {
			return nil
		}
		if err := b.current.close(); err != nil {
			b.fenced = true
			return errors.Join(ErrCleanup, err)
		}
		b.current = nil
	}
	next, err := b.open()
	if err != nil {
		if errors.Is(err, ErrCleanup) {
			b.fenced = true
		}
		return err
	}
	b.current = next
	if err := b.authorize(); err != nil {
		return err
	}
	err = next.active()
	if errors.Is(err, ErrInactive) {
		return desktopPreparing(err)
	}
	return err
}

func (b *interactiveBinding) close() error {
	if b.fenced {
		return ErrCleanup
	}
	if b.current == nil {
		return nil
	}
	err := b.current.close()
	if err != nil {
		b.fenced = true
	} else {
		b.current = nil
	}
	return err
}
