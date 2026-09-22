//go:build windows && desktop

package main

import "github.com/0disoft/krasis/internal/workspacehost"

// Preserve the update entry point accepted by existing agents while sharing
// the exact same Workspace UI and lifecycle as krasis-workspace.exe.
func main() { workspacehost.Run() }
