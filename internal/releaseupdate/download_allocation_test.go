package releaseupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"testing/iotest"

	"github.com/0disoft/krasis/internal/updatebundle"
)

type archiveReadCloser struct {
	io.Reader
	read   int
	closed bool
}

func (r *archiveReadCloser) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.read += n
	return n, err
}

func (r *archiveReadCloser) Close() error { r.closed = true; return nil }

type archiveReadFunc func([]byte) (int, error)

func (f archiveReadFunc) Read(p []byte) (int, error) { return f(p) }

func TestFetchArchiveUsesManifestSizeAndBoundsActualRead(t *testing.T) {
	const size = 3
	m := manifest()
	digest := sha256.Sum256([]byte("aaa"))
	m.Size, m.SHA256 = size, hex.EncodeToString(digest[:])
	for _, declared := range []int64{-1, 0, size - 1, size, size + 1} {
		for _, length := range []int{0, size - 1, size, size + 1, 40} {
			t.Run(fmt.Sprintf("declared=%d/actual=%d", declared, length), func(t *testing.T) {
				want := bytes.Repeat([]byte("a"), length)
				body := &archiveReadCloser{Reader: iotest.OneByteReader(bytes.NewReader(want))}
				calls := 0
				c := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
					calls++
					return &http.Response{StatusCode: 200, ContentLength: declared, Body: body}, nil
				})}
				data, err := DownloadArchiveBytes(context.Background(), c, m)
				if length == size && declared <= size {
					if err != nil || !bytes.Equal(data, want) {
						t.Fatalf("valid body rejected: %q, %v", data, err)
					}
				} else if !errors.Is(err, ErrInvalid) || data != nil {
					t.Fatalf("invalid body returned: %q, %v", data, err)
				}
				read := min(length, size+1)
				if declared > size {
					read = 0 // Reject headers before allocating or reading the body.
				}
				if calls != 1 || !body.closed || body.read != read {
					t.Fatalf("calls=%d closed=%v read=%d want=%d", calls, body.closed, body.read, read)
				}
			})
		}
	}
}

func TestFullDownloadPreservesBytesOnTruncatedArchive(t *testing.T) {
	m := manifest()
	m.Size = 3
	c := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, ContentLength: -1, Body: io.NopCloser(bytes.NewBufferString("ab"))}, nil
	})}
	if bundle, data, err := DownloadArchive(context.Background(), c, m); !errors.Is(err, ErrInvalid) || bundle != nil || string(data) != "ab" {
		t.Fatalf("full download changed validation-failure result: %q, %v", data, err)
	}
}

func TestFetchArchivePreservesFinalReadErrorsAndCancellation(t *testing.T) {
	failure := errors.New("body read failed")
	for _, terminal := range []error{io.EOF, io.ErrUnexpectedEOF, failure, context.DeadlineExceeded, context.Canceled} {
		t.Run(terminal.Error(), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			body := &archiveReadCloser{Reader: archiveReadFunc(func(p []byte) (int, error) {
				if terminal == context.Canceled {
					cancel()
					return copy(p, "abc"), ctx.Err()
				}
				return copy(p, "abc"), terminal
			})}
			c := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, ContentLength: 3, Body: body}, nil
			})}
			data, err := fetchArchive(ctx, c, "https://github.com/asset", 3)
			if terminal == io.EOF {
				if err != nil || string(data) != "abc" {
					t.Fatalf("data with EOF rejected: %q, %v", data, err)
				}
			} else if !errors.Is(err, terminal) || data != nil {
				t.Fatalf("final read error lost: %q, %v", data, err)
			}
			if !body.closed {
				t.Fatal("response body left open")
			}
		})
	}
}

func TestFullDownloadRejectsInvalidSizeBeforeNetwork(t *testing.T) {
	c := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid archive size contacted network")
		return nil, nil
	})}
	for _, size := range []int64{-1, 0, updatebundle.MaxArchiveBytes + 1, 1<<63 - 1} {
		m := manifest()
		m.Size = size
		if bundle, data, err := DownloadArchive(context.Background(), c, m); !errors.Is(err, ErrInvalid) || bundle != nil || data != nil {
			t.Fatalf("size=%d accepted: %v", size, err)
		}
	}
}

// Exercise HTTP body collection and manifest matching without ZIP expansion,
// disk access or an external server. These bytes are never staged or executed.
func BenchmarkDownloadArchiveAllocation(b *testing.B) {
	archive := bytes.Repeat([]byte("0123456789abcdef"), 1<<20)
	digest := sha256.Sum256(archive)
	m := manifest()
	m.Size, m.SHA256 = int64(len(archive)), hex.EncodeToString(digest[:])
	for _, length := range []struct {
		name string
		size int64
	}{{"known_length", m.Size}, {"chunked", -1}} {
		c := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, ContentLength: length.size, Body: io.NopCloser(bytes.NewReader(archive))}, nil
		})}
		for _, strategy := range []struct {
			name     string
			download func(context.Context, *http.Client, Manifest) ([]byte, error)
		}{{"read_all", func(ctx context.Context, c *http.Client, m Manifest) ([]byte, error) {
			data, err := fetch(ctx, c, m.URL, m.Size)
			if err == nil && !archiveMatchesManifest(data, m) {
				err = ErrInvalid
			}
			return data, err
		}}, {"sized", DownloadArchiveBytes}} {
			b.Run(length.name+"/"+strategy.name, func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(m.Size)
				for b.Loop() {
					if _, err := strategy.download(context.Background(), c, m); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
