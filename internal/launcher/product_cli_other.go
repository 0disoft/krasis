//go:build !linux

package launcher

import (
	"context"
	"io"

	"github.com/0disoft/krasis/internal/productcli"
)

func RunProductCLI(ctx context.Context, args []string, stdout, stderr io.Writer, localData string) int {
	return productcli.Run(ctx, args, stdout, stderr, localData)
}
