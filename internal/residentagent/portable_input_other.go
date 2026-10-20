//go:build !windows

package residentagent

import (
	"context"

	"github.com/0disoft/krasis/internal/wininput"
)

func portableProductInput(context.Context, int32, int32) (*wininput.InputAdapter, productSessionGuard, error) {
	return nil, nil, wininput.ErrUnsupportedInput
}
