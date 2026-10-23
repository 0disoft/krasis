//go:build windows

package securefs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClosedDirectoryRejectsOperationsBeforeNativeAccess(t *testing.T) {
	directory, err := OpenDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := directory.Close(); err != nil {
		t.Fatal(err)
	}
	if directory.Handle() != 0 {
		t.Error("closed directory exposed a reusable native handle")
	}
	check := func(err error) {
		t.Helper()
		if !errors.Is(err, os.ErrClosed) {
			t.Errorf("closed operation returned %v, want os.ErrClosed", err)
		}
	}
	file, err := directory.OpenObserved("file", func(string) { t.Error("closed directory reached native open") })
	if file != nil {
		file.Close()
	}
	check(err)
	file, err = directory.CreateExclusive("file", 0600)
	if file != nil {
		file.Close()
	}
	check(err)
	child, err := directory.CreateDirectory("child", 0700)
	if child != nil {
		child.Close()
	}
	check(err)
	_, err = directory.ReadNames()
	check(err)
	check(directory.Remove("file"))
	check(directory.RemoveSelf())
	check(directory.Replace("file", "other"))
	if err := directory.Close(); err != nil {
		t.Fatal("repeated close failed", err)
	}
}

func TestDirectoryCloseWaitsForChildOpenButNotIndependentFile(t *testing.T) {
	path := t.TempDir()
	if err := os.WriteFile(filepath.Join(path, "file"), []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	directory, err := OpenReadDirectory(path)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	entered, release := make(chan struct{}), make(chan struct{})
	metadata, releaseMetadata := make(chan struct{}), make(chan struct{})
	opened := make(chan *os.File, 1)
	go func() {
		file, err := directory.OpenObserved("file", func(stage string) {
			switch stage {
			case "native":
				close(entered)
				<-release
			case "info":
				close(metadata)
				<-releaseMetadata
			}
		})
		if err != nil {
			t.Error(err)
		}
		opened <- file
	}()
	<-entered
	closing, closed := make(chan struct{}), make(chan error, 1)
	go func() { close(closing); closed <- directory.Close() }()
	<-closing
	select {
	case err := <-closed:
		t.Error("close invalidated a root handle still in use", err)
		closed <- err
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case <-metadata:
	case file := <-opened:
		if file != nil {
			file.Close()
		}
		close(releaseMetadata)
		t.Fatal("child open failed before metadata validation")
	}
	// Child metadata can be slow. It owns a different handle, so it must not
	// keep the root alive or delay root cleanup.
	select {
	case err := <-closed:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(2 * time.Second):
		t.Error("directory close waited for independent child metadata")
	}
	close(releaseMetadata)
	file := <-opened
	if file == nil {
		t.Fatal("in-flight child open failed")
	}
	defer file.Close()
	data := make([]byte, 5)
	if _, err := file.ReadAt(data, 0); err != nil || string(data) != "owned" {
		t.Fatal("closing directory invalidated independent child", string(data), err)
	}
}
