//go:build windows

package productlog

import (
	"github.com/0disoft/krasis/internal/inputworker"
	"github.com/0disoft/krasis/internal/logindiag"
)

func readLoginRuntime() (*logindiag.Runtime, error) { return inputworker.ReadLoginRuntime() }
