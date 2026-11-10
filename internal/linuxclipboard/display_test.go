package linuxclipboard

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"testing"
)

func TestDisplayAddresses(t *testing.T) {
	for _, tc := range []struct {
		input, network, address string
		screen                  int
	}{
		{":0", "unix", "/tmp/.X11-unix/X0", 0}, {"unix/:2.1", "unix", "/tmp/.X11-unix/X2", 1},
	} {
		d, err := parseDisplay(tc.input)
		if err != nil || d.network != tc.network || d.address != tc.address || d.screen != tc.screen {
			t.Fatalf("%q: %+v %v", tc.input, d, err)
		}
	}
	for _, input := range []string{"", ":-1", ":65536", ":1.-1", ":1.256", ":1.2.3", "host", "bad/host:0", ":1.", "localhost:10", "tcp/host:0", "[::1]:1", "unix:0"} {
		if _, err := parseDisplay(input); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}

func TestAuthorityUsesCorrectDisplayAndRejectsTruncation(t *testing.T) {
	var b bytes.Buffer
	record := func(display, cookie string) {
		_ = binary.Write(&b, binary.BigEndian, uint16(256))
		for _, field := range []string{"host", display, "MIT-MAGIC-COOKIE-1", cookie} {
			_ = binary.Write(&b, binary.BigEndian, uint16(len(field)))
			b.WriteString(field)
		}
	}
	record("1", strings.Repeat("x", 16))
	record("10", strings.Repeat("y", 16))
	got, err := authorityCookie(bytes.NewReader(b.Bytes()), "host", "10")
	if err != nil || got != strings.Repeat("79", 16) {
		t.Fatal("wrong display's auth cookie", err)
	}
	for end := 0; end < b.Len(); end++ {
		if _, err := authorityCookie(bytes.NewReader(b.Bytes()[:end]), "host", "10"); err == nil {
			t.Fatalf("accepted truncated record at %d", end)
		}
	}
	if _, err := authorityCookie(bytes.NewReader([]byte{1, 0, 255, 255}), "host", "0"); err == nil || err == io.EOF {
		t.Fatal("oversized field not rejected")
	}
}

func TestLatin1ConversionsDoNotLoseUnicode(t *testing.T) {
	text := latin1Text([]byte{'c', 'a', 'f', 0xe9})
	if text != "café" {
		t.Fatal(text)
	}
	data, ok := textLatin1(text)
	if !ok || !bytes.Equal(data, []byte{'c', 'a', 'f', 0xe9}) {
		t.Fatal("Latin1 roundtrip changed")
	}
	if _, ok := textLatin1("제로디 🐧"); ok {
		t.Fatal("Unicode silently truncated to Latin1")
	}
}
