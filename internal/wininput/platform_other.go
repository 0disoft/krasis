//go:build !windows

package wininput

import (
	"context"
	"fmt"

	"github.com/0disoft/krasis/internal/lab"
)

type Platform struct{}

func NewPlatform() *Platform {
	return &Platform{}
}

func (p *Platform) Diagnose(context.Context) (lab.Diagnostics, error) {
	return lab.Diagnostics{}, fmt.Errorf("%w: krasis-lab requires Windows", lab.ErrUnsupported)
}

func (p *Platform) Capture(context.Context, lab.CaptureOptions, func(lab.CaptureSnapshot)) error {
	return fmt.Errorf("%w: krasis-lab requires Windows", lab.ErrUnsupported)
}
