//go:build windows

package launcherui

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// WalkDir reads and sorts an entire directory before visiting its children.
// Reserve entries when discovered, including pending siblings, so even a wide
// or deeply nested rejected tree retains at most the entry budget plus one.
// Accepted trees keep WalkDir's lexical, depth-first order. The packing callback
// validates each entry (including reparse points) before we enumerate beneath it.
func walkClipboardFiles(root string, remaining int, visit fs.WalkDirFunc) error {
	if remaining <= 0 {
		return errClipboardFileCount
	}
	info, err := os.Lstat(root)
	if err != nil {
		return visit(root, nil, err)
	}
	remaining-- // Reserve the root; descendants are reserved during enumeration.
	var walk func(string, fs.DirEntry) error
	walk = func(current string, entry fs.DirEntry) error {
		if err := visit(current, entry, nil); err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		directory, err := os.Open(current)
		if err != nil {
			return visit(current, entry, err)
		}
		// Always positive: ReadDir(0) would read the whole directory.
		children, readErr := directory.ReadDir(remaining + 1)
		closeErr := directory.Close()
		if readErr != nil && readErr != io.EOF {
			return visit(current, entry, readErr)
		}
		if closeErr != nil {
			return visit(current, entry, closeErr)
		}
		if len(children) > remaining {
			return errClipboardFileCount
		}
		remaining -= len(children)
		slices.SortFunc(children, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
		for _, child := range children {
			if err := walk(filepath.Join(current, child.Name()), child); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(root, fs.FileInfoToDirEntry(info))
}
