//go:build !desktop

package main

import (
	"context"
	"os"

	"github.com/0disoft/krasis/internal/launcher"
	"github.com/0disoft/krasis/internal/productpath"
)

func main() {
	localData, _ := productpath.LocalData()
	os.Exit(launcher.RunProductCLI(context.Background(), os.Args[1:], os.Stdout, os.Stderr, localData))
}
