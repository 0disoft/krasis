//go:build windows && desktop

package frontend

import "embed"

// Native assets are built separately so a preview build cannot silently replace
// the production renderer with simulated state.
//
//go:embed all:dist-native
var NativeAssets embed.FS
