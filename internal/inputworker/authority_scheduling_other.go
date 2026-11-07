//go:build !windows

package inputworker

import "context"

func startAuthorityQuery(_ context.Context, _ authorityQueryKind, work func()) { go work() }
