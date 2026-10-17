package peerupdate

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/updatebundle"
)

func queuedFileOffer(t *testing.T, m *Manager, peer string) *offer {
	t.Helper()
	id := randomID()
	name := archiveNameFor(id)
	file, err := m.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	const data = "independent file-backed readers"
	if _, err = file.WriteString(data); err != nil {
		t.Fatal(err)
	}
	current := &offer{id: id, peer: peer, file: file, name: name, size: int64(len(data)), expires: time.Now().Add(time.Minute)}
	m.out = current
	m.status = Status{State: "QUEUED", ID: id, Peer: peer}
	return current
}

func TestRetiredOfferWaitsForLastIndependentReaderAndCannotReviveProgress(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	current := queuedFileOffer(t, m, peer)
	first, second := m.offered(peer), m.offered(peer)
	if first == nil || second == nil {
		t.Fatal("missing file readers")
	}
	firstByte := make([]byte, 1)
	if _, err := first.reader.Read(firstByte); err != nil {
		t.Fatal(err)
	}
	all, err := io.ReadAll(second.reader)
	if err != nil || string(all) != "independent file-backed readers" {
		t.Fatal("shared read offset", err)
	}
	m.sent(current.id, "DELIVERED")
	m.progress(peer, current.id, "SENDING", 1, current.size)
	if m.Status().State != "DELIVERED" {
		t.Fatal("late progress revived retired transfer")
	}
	if _, err := m.root.Stat(current.name); err != nil {
		t.Fatal("retired file removed with live readers", err)
	}
	first.Close()
	first.Close()
	if _, err := current.file.Stat(); err != nil {
		t.Fatal("first reader closed shared file", err)
	}
	second.Close()
	if _, err := m.root.Stat(current.name); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("last reader retained archive", err)
	}
	if _, err := current.file.ReadAt(make([]byte, 1), 0); !errors.Is(err, os.ErrClosed) {
		t.Fatal("retained archive handle", err)
	}
}

func TestOfferExpiresWithoutStatusPollingAndShutdownDiscardsQueuedFile(t *testing.T) {
	for _, scenario := range []string{"expiry", "close"} {
		t.Run(scenario, func(t *testing.T) {
			m := testManager(t)
			current := queuedFileOffer(t, m, strings.Repeat("a", 64))
			if scenario == "close" {
				if err := m.Close(); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(m.path + "/" + current.name); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("shutdown retained archive", err)
				}
			} else {
				m.mu.Lock()
				current.expires = time.Now().Add(10 * time.Millisecond)
				m.armOfferExpiryLocked(current)
				m.mu.Unlock()
				deadline := time.Now().Add(2 * time.Second)
				for {
					m.mu.Lock()
					expired := m.out == nil && m.status.State == "EXPIRED"
					m.mu.Unlock()
					if expired {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("offer needs UI polling to expire")
					}
					time.Sleep(time.Millisecond)
				}
				if _, err := m.root.Stat(current.name); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("expiry retained archive", err)
				}
			}
		})
	}
}

func TestReceiveStreamsToFileAndAlwaysRemovesTemporaryArchive(t *testing.T) {
	for _, scenario := range []string{"success", "hash", "read", "limit"} {
		t.Run(scenario, func(t *testing.T) {
			m := testManager(t)
			peer := strings.Repeat("a", 64)
			m.allowed[peer] = true
			size := int64(16 << 20)
			if scenario == "limit" {
				size = updatebundle.MaxArchiveBytes + 1
			}
			hash := sha256.New()
			io.Copy(hash, io.LimitReader(zeroArchiveReader{}, size))
			expected := fmt.Sprintf("%x", hash.Sum(nil))
			if scenario == "hash" {
				expected = strings.Repeat("0", 64)
			}
			var archive *os.File
			bundle := &fakeBundle{generation: 22}
			m.validate = func(source io.ReaderAt, n int64) (stagedBundle, error) {
				if scenario != "success" {
					t.Fatal("validated failed transfer")
				}
				var ok bool
				archive, ok = source.(*os.File)
				if !ok || n != size {
					t.Fatal("receiver retained an in-memory archive")
				}
				return bundle, nil
			}
			var reader io.Reader = io.LimitReader(zeroArchiveReader{}, size)
			if scenario == "read" {
				reader = io.MultiReader(strings.NewReader("partial"), failedArchiveReader{})
			}
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			err := m.receive(peer, randomID(), expected, reader, func() bool { return true })
			runtime.ReadMemStats(&after)
			if (err == nil) != (scenario == "success") {
				t.Fatal(scenario, err)
			}
			if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 4<<20 {
				t.Fatalf("receive allocated %d bytes for %d-byte archive", allocated, size)
			}
			if scenario == "success" {
				if !bundle.staged || bundle.closed != 1 {
					t.Fatal("bundle cleanup ownership", bundle)
				}
				if _, err := archive.ReadAt(make([]byte, 1), 0); !errors.Is(err, os.ErrClosed) {
					t.Fatal("receive retained file handle", err)
				}
			}
			listing, err := m.root.Open(".")
			if err != nil {
				t.Fatal(err)
			}
			entries, readErr := listing.ReadDir(-1)
			listing.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
			if len(entries) != 0 {
				t.Fatal("temporary transfer files retained", entries)
			}
		})
	}
}

type zeroArchiveReader struct{}

func (zeroArchiveReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

type failedArchiveReader struct{}

func (failedArchiveReader) Read([]byte) (int, error) { return 0, errors.New("connection lost") }

func TestFailedPrepareAndAbandonedArchiveCleanup(t *testing.T) {
	m := testManager(t)
	if err := m.Prepare(strings.Repeat("a", 64), t.TempDir()); err == nil {
		t.Fatal("empty source became an offer")
	}
	listing, err := m.root.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := listing.ReadDir(-1)
	listing.Close()
	if err != nil || len(entries) != 0 {
		t.Fatal("failed prepare retained temporary archive", err, entries)
	}
	now := time.Now()
	old := archiveNameFor(randomID())
	recent := archiveNameFor(randomID())
	for _, name := range []string{old, recent, "archive-unknown.zip"} {
		if err := m.root.WriteFile(name, []byte("abandoned"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(filepath.Join(m.path, old), now.Add(-48*time.Hour), now.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	n, err := m.prune(now, func() ([]string, error) { return nil, nil })
	if err != nil || n != 1 {
		t.Fatal("old archive cleanup", n, err)
	}
	for _, name := range []string{recent, "archive-unknown.zip"} {
		if _, err := m.root.Stat(name); err != nil {
			t.Fatal("removed recent or unknown archive", err)
		}
	}
}

func TestFileBackedWorkspaceUpdatePackage(t *testing.T) {
	source := os.Getenv("KRASIS_WORKSPACE_UPDATE")
	if source == "" {
		t.Skip("real package check only")
	}
	for _, direction := range []string{"upload", "download"} {
		t.Run(direction, func(t *testing.T) {
			sender, receiver := testManager(t), testManager(t)
			peer := strings.Repeat("a", 64)
			receiver.allowed[peer] = true
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			if err := sender.Prepare(peer, source); err != nil {
				t.Fatal(err)
			}
			id := sender.Status().ID
			if direction == "upload" {
				server := httptest.NewServer(receiver.handler(peer, func() bool { return true }))
				defer server.Close()
				lease := sender.offered(peer)
				if lease == nil {
					t.Fatal("missing file offer")
				}
				defer lease.Close()
				request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/bundle", &offerRequestBody{Reader: lease.reader, lease: lease})
				if err != nil {
					t.Fatal(err)
				}
				request.ContentLength = lease.size
				request.Header.Set("Content-Type", "application/zip")
				request.Header.Set("X-Krasis-ID", lease.id)
				request.Header.Set("X-Krasis-SHA256", lease.hash)
				response, err := server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if response.StatusCode != 200 {
					t.Fatal("upload failed", response.StatusCode, response.Header.Get("X-Krasis-Result"))
				}
				sender.sent(id, "DELIVERED")
			} else {
				server := httptest.NewServer(sender.handler(peer, func() bool { return true }))
				defer server.Close()
				response, err := server.Client().Get(server.URL + "/v1/bundle")
				if err != nil {
					t.Fatal(err)
				}
				if response.StatusCode != 200 {
					response.Body.Close()
					t.Fatal(response.StatusCode)
				}
				err = receiver.receive(peer, response.Header.Get("X-Krasis-ID"), response.Header.Get("X-Krasis-SHA256"), response.Body, func() bool { return true })
				response.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				sender.sent(id, "DELIVERED")
				receiver.ready(id)
			}
			if receiver.Status().State != "READY" || sender.Status().State != "DELIVERED" {
				t.Fatal(sender.Status(), receiver.Status())
			}
			path, revision, err := receiver.Claim(id, peer)
			if err != nil || revision != os.Getenv("KRASIS_UPDATE_REVISION") {
				t.Fatal("claim/provenance mismatch", revision, err)
			}
			if _, err := os.Stat(filepath.Join(path, "krasis-install.exe")); err != nil {
				t.Fatal("installer lost", err)
			}
			runtime.ReadMemStats(&after)
			t.Logf("file-backed %s completed; total Go allocation across packing, validation and HTTP: %d bytes", direction, after.TotalAlloc-before.TotalAlloc)
		})
	}
}
