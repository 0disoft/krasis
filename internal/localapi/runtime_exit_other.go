//go:build !windows

package localapi

import "context"

func pinRuntimeExit(context.Context, *Client, string) (runtimeExit, error) { return nil, nil }
