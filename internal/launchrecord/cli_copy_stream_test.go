package launchrecord

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/0disoft/krasis/internal/productidentity"
)

type cliChunkWriter struct {
	io.Writer
	writes, largest int
}

func (w *cliChunkWriter) Write(data []byte) (int, error) {
	w.writes++
	w.largest = max(w.largest, len(data))
	return w.Writer.Write(data)
}

func TestCLICopyStreamPreservesBytesInBoundedWriteThroughChunks(t *testing.T) {
	data := bytes.Repeat([]byte{37}, 2*cliCopyBufferSize+11)
	digest := sha256.New()
	output := &cliChunkWriter{Writer: digest}
	if err := copyCLIStream(output, bytes.NewReader(data), int64(len(data))); err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(data)
	if !bytes.Equal(digest.Sum(nil), want[:]) || output.writes != 3 || output.largest != cliCopyBufferSize {
		t.Fatal("CLI bytes changed or synchronous writes lost their bounded chunk size", output.writes, output.largest)
	}
}

func TestCLIStreamFailureCannotPublishPartialRecoveryImage(t *testing.T) {
	cause := errors.New("source read failed")
	for _, tc := range []struct {
		name   string
		source io.Reader
		size   int64
		want   error
	}{
		{"truncated", strings.NewReader("MZ"), 3, ErrInvalid},
		{"grown", strings.NewReader("MZ-extra"), 2, ErrInvalid},
		{"read-error", io.MultiReader(strings.NewReader("MZ"), iotest.ErrReader(cause)), 3, cause},
		{"empty-size", strings.NewReader(""), 0, ErrInvalid},
		{"oversized", strings.NewReader("MZ"), maxExecutable + 1, ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			destination, err := productidentity.PrepareLaunch(base)
			if err != nil {
				t.Fatal(err)
			}
			defer destination.Close()
			original := []byte("MZ-keep")
			if err := replace(destination, "krasis-cli.exe", original); err != nil {
				t.Fatal(err)
			}
			err = replaceWith(destination, "krasis-cli.exe", func(file *os.File) error {
				return copyCLIStream(file, tc.source, tc.size)
			})
			if !errors.Is(err, tc.want) {
				t.Fatal("partial stream was published or its error lost", err)
			}
			path, err := Directory(base)
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join(path, "krasis-cli.exe"))
			if err != nil || !bytes.Equal(got, original) {
				t.Fatal("failed copy replaced the recovery CLI", err)
			}
			if err := destination.Close(); err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".launch-") {
					t.Fatal("failed stream left a temporary executable")
				}
			}
		})
	}
}

func TestCLICopyStreamRejectsShortDestinationWrite(t *testing.T) {
	if err := copyCLIStream(cliShortWriter{}, strings.NewReader("MZ"), 2); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal("short destination write was accepted", err)
	}
}

type cliShortWriter struct{}

func (cliShortWriter) Write(data []byte) (int, error) { return len(data) - 1, nil }
