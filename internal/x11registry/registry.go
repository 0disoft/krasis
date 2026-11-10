// Package x11registry registers XGB extension decoders once per process.
package x11registry

import (
	"errors"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xfixes"
	"github.com/jezek/xgb/xproto"
)

type registry struct {
	sync.Mutex
	initialized    bool
	event, failure byte
}

var shared registry

// record permits only the first registration to mutate XGB's global maps.
// Subsequent connections must have identical decoder bases. The mutex protects
// metadata and initialization; it is never used around X11 I/O or event reads.
func (r *registry) record(event, failure byte, register func()) error {
	r.Lock()
	defer r.Unlock()
	if r.initialized {
		if r.event != event || r.failure != failure {
			return errors.New("XFIXES event layout differs from existing display")
		}
		return nil
	}
	register()
	r.initialized, r.event, r.failure = true, event, failure
	return nil
}

// InitXfixes replaces xfixes.Init for every X11 client in this process. Call it
// before subscribing to extension events. XGB's generated Init rewrites global
// decoder maps on each connection, racing with an existing connection's reader.
func InitXfixes(c *xgb.Conn) error {
	if c == nil {
		return errors.New("X11 connection is required")
	}
	r, err := xproto.QueryExtension(c, 6, "XFIXES").Reply()
	if err != nil {
		return err
	}
	if r == nil || !r.Present {
		return errors.New("X11 clipboard/input requires XFIXES")
	}
	if err := shared.record(r.FirstEvent, r.FirstError, func() {
		for n, f := range xgb.NewExtEventFuncs["XFIXES"] {
			xgb.NewEventFuncs[int(r.FirstEvent)+n] = f
		}
		for n, f := range xgb.NewExtErrorFuncs["XFIXES"] {
			xgb.NewErrorFuncs[int(r.FirstError)+n] = f
		}
	}); err != nil {
		return err
	}
	c.ExtLock.Lock()
	c.Extensions["XFIXES"] = r.MajorOpcode
	c.ExtLock.Unlock()
	version, err := xfixes.QueryVersion(c, 5, 0).Reply()
	if err != nil {
		return err
	}
	if version == nil || version.MajorVersion < 1 {
		return errors.New("XFIXES selection events unavailable")
	}
	return nil
}
