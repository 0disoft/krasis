//go:build windows

package inputworker

import (
	"crypto/sha256"
	"encoding/binary"
	"io"
	"os"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestNativeInspectOwnProcessWithoutInput(t *testing.T) {
	identity, err := inspectProcess(windows.GetCurrentProcessId())
	if err != nil {
		t.Fatal("own process token metadata inspection failed")
	}
	defer identity.close()
	if identity.image == "" || identity.handle == 0 || identity.token == 0 || identity.integrity == 0 {
		t.Fatal("incomplete native identity")
	}
	if _, err := tokenLogon(identity.token); err != nil {
		t.Fatal("token logon identity inspection failed")
	}
	if unsafe.Sizeof(tokenStatistics{}) != 56 {
		t.Fatal("TOKEN_STATISTICS layout mismatch")
	}
}

func TestNativeImageDigestPinAndWriteLockWithoutInput(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatal("test executable unavailable")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal("test executable cannot be read")
	}
	hash := sha256.New()
	_, err = io.Copy(hash, file)
	_ = file.Close()
	if err != nil {
		t.Fatal("test digest unavailable")
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	locked, err := lockImage(path, digest)
	if err != nil {
		t.Fatal("native executable digest pin failed")
	}
	defer locked.Close()
	digest[0] ^= 1
	if bad, err := lockImage(path, digest); err != ErrAuthority {
		if bad != nil {
			_ = bad.Close()
		}
		t.Fatal("incorrect digest accepted")
	}
}

func TestAgentPolicyRejectsDifferentSessionsAndTokenClasses(t *testing.T) {
	valid := processIdentity{session: 1, integrity: 0x2000}
	if !validAgentIdentity(&valid, 1) {
		t.Fatal("baseline facts rejected")
	}
	for _, bad := range []processIdentity{
		{session: 2, integrity: 0x2000}, {session: 1, integrity: 0x1000}, {session: 1, integrity: 0x4000},
		{session: 1, integrity: 0x2000, system: true}, {session: 1, integrity: 0x2000, restricted: true}, {session: 1, integrity: 0x2000, appContainer: true},
	} {
		if validAgentIdentity(&bad, 1) {
			t.Fatal("untrusted token class accepted")
		}
	}
	if validAgentIdentity(nil, 1) || validAgentIdentity(&valid, 0) || validAgentIdentity(&valid, 0xffffffff) {
		t.Fatal("unknown console accepted")
	}
}

func TestNoPipeOrNoDigestCannotAuthorize(t *testing.T) {
	for _, check := range []func(uintptr, [sha256.Size]byte) (*Authority, error){AuthenticateAgent, AuthenticateService} {
		if proof, err := check(0, [sha256.Size]byte{1}); proof != nil || err != ErrAuthority {
			t.Fatal("missing pipe accepted")
		}
		if proof, err := check(1, [sha256.Size]byte{}); proof != nil || err != ErrAuthority {
			t.Fatal("missing digest accepted")
		}
	}
}

func TestIntegrityRIDRejectsPointersOutsideTokenBuffer(t *testing.T) {
	const offset = 16
	buffer := make([]byte, offset+12)
	address := uintptr(unsafe.Pointer(&buffer[offset]))
	writePointer := func(value uintptr) {
		if unsafe.Sizeof(uintptr(0)) == 8 {
			binary.LittleEndian.PutUint64(buffer[:8], uint64(value))
		} else {
			binary.LittleEndian.PutUint32(buffer[:4], uint32(value))
		}
	}
	writePointer(address)
	buffer[offset], buffer[offset+1], buffer[offset+7] = 1, 1, 16
	binary.LittleEndian.PutUint32(buffer[offset+8:], 0x4000)
	if rid, err := integrityRID(buffer); err != nil || rid != 0x4000 {
		t.Fatal("valid mandatory label rejected")
	}
	for _, bad := range []uintptr{0, address - 17, address + 1, address + 12} {
		writePointer(bad)
		if _, err := integrityRID(buffer); err != ErrAuthority {
			t.Fatal("out-of-buffer SID accepted")
		}
	}
	writePointer(address)
	buffer[offset+7] = 5
	if _, err := integrityRID(buffer); err != ErrAuthority {
		t.Fatal("non-integrity SID accepted")
	}
}
