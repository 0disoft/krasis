package productpanel

import (
	"context"
	"errors"
	"testing"
)

func TestRecoveryPromotionRequiresSuccessfulStartup(t *testing.T) {
	for _, failure := range []error{errors.New("bad bundle"), nil} {
		promotions := 0
		b := NewDesktopLifecycle(context.Background(), nil, func(context.Context) error { return failure }, nil)
		b.StartupReady = func() { promotions++ }
		err := b.startup(context.Background())
		if failure != nil && (err == nil || promotions != 0) || failure == nil && (err != nil || promotions != 1) {
			t.Fatal("recovery was promoted before a successful start", err, promotions)
		}
	}
}
