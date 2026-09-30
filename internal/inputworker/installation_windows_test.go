//go:build windows

package inputworker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"golang.org/x/sys/windows"
)

func TestInstallationRecordRejectsUnknownTrailingAndMissingTrust(t *testing.T) {
	agent := sha256.Sum256([]byte("installation-test-agent"))
	service := sha256.Sum256([]byte("installation-test-service"))
	record := installationRecord{Schema: "krasis.input.install/v1", Agent: hex.EncodeToString(agent[:]), Service: hex.EncodeToString(service[:])}
	data, _ := json.Marshal(record)
	a, s, err := decodeInstallation(data)
	if err != nil || a != agent || s != service {
		t.Fatal("installation digest roundtrip failed")
	}
	for _, bad := range [][]byte{
		append(append([]byte{}, data...), []byte(" {}")...),
		[]byte(`{"schemaVersion":"krasis.input.install/v1","path":"user-selected.exe"}`),
		[]byte(`{"schemaVersion":"other","agentSHA256":"00","serviceSHA256":"00"}`),
	} {
		if _, _, err := decodeInstallation(bad); err != ErrAuthority {
			t.Fatal("untrusted installation record accepted")
		}
	}
	var zero Installation
	if _, _, err := zero.digests(); err != ErrAuthority {
		t.Fatal("zero installation authorizes service")
	}
}

func TestProtectedInstallDescriptorRequiresAdministratorOwnerAndNoUserWrites(t *testing.T) {
	for _, example := range []struct {
		descriptor string
		safe       bool
	}{
		{"O:SYG:SYD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;GRGX;;;BU)", true},
		{"O:BAG:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FRFX;;;BU)", true},
		{"O:SYG:SYD:P(A;;FA;;;SY)(A;;FA;;;BU)", false},
		{"O:SYG:SYD:P(A;;0x40;;;BU)", false},
		{"O:SYG:SYD:P(A;;WD;;;BU)", false},
		{"O:BUG:BUD:P(A;;FR;;;BU)", false},
		{"O:SYG:SYD:NO_ACCESS_CONTROL", false},
	} {
		descriptor, err := windows.SecurityDescriptorFromString(example.descriptor)
		if err != nil {
			t.Fatal("test descriptor invalid")
		}
		if (protectedDescriptor(descriptor) == nil) != example.safe {
			t.Fatal("installation access classification mismatch")
		}
	}
}
