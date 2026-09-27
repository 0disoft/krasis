//go:build !desktop

package main

import (
	"context"
	"os"

	"github.com/0disoft/krasis/internal/productcli"
)

func main() {
	os.Exit(productcli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, os.Getenv("LOCALAPPDATA")))
}
