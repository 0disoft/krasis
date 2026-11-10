package wininput

import (
	"context"
	"fmt"
	"github.com/0disoft/krasis/internal/lab"
	"github.com/0disoft/krasis/internal/x11desktop"
	"runtime"
	"time"
)

type Platform struct{}

func NewPlatform() *Platform { return &Platform{} }
func (*Platform) Diagnose(ctx context.Context) (lab.Diagnostics, error) {
	d, err := x11desktop.Open(ctx)
	if err != nil {
		return lab.Diagnostics{}, fmt.Errorf("%w: %w", ErrUnsupportedInput, err)
	}
	defer d.Close()
	g, err := d.Geometry(ctx)
	if err != nil {
		return lab.Diagnostics{}, err
	}
	return lab.Diagnostics{SchemaVersion: "1", OS: runtime.GOOS, Architecture: runtime.GOARCH, Display: lab.DisplayInfo{MonitorCount: g.Count, Width: g.Width, Height: g.Height, MouseButtons: 5}}, nil
}
func (*Platform) Capture(context.Context, lab.CaptureOptions, func(lab.CaptureSnapshot)) error {
	return fmt.Errorf("%w: use the Linux product source hook", lab.ErrUnsupported)
}
func (*Platform) SelfTestInject(context.Context) (lab.SelfTestResult, error) {
	return lab.SelfTestResult{}, fmt.Errorf("%w: use the opt-in X11 integration smoke", lab.ErrUnsupported)
}
func CurrentDisplayGeometry() DisplayGeometry {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	g, err := x11desktop.QueryGeometry(ctx)
	if err != nil {
		return DisplayGeometry{}
	}
	return DisplayGeometry{Count: g.Count, Width: g.Width, Height: g.Height}
}
func DisplayCoordinates() (uint32, uint32, int32) {
	g := CurrentDisplayGeometry()
	return uint32(g.Width), uint32(g.Height), 0
}
