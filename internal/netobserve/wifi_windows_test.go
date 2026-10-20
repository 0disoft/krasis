//go:build windows

package netobserve

import (
	"testing"
	"unsafe"
)

func TestWiFiNativeABISizes(t *testing.T) {
	if unsafe.Sizeof(wlanInterface{}) != 532 || unsafe.Sizeof(wlanAssociation{}) != 68 || unsafe.Sizeof(wlanConnection{}) != 604 || unsafe.Offsetof(wlanConnection{}.association) != 520 {
		t.Fatal("WLAN API ABI mismatch")
	}
	if !hresultOK(0) || !hresultOK(1) || hresultOK(0x80010106) {
		t.Fatal("COM success/S_FALSE handling wrong")
	}
}

func TestWiFiProviderHonorsAccessAndClosesNativeOwner(t *testing.T) {
	for _, kind := range []string{"connected", "permission", "query-denied", "invalid-ssid", "empty-ssid", "invalid-ap", "no-interfaces", "disconnected", "enumeration-failed", "close-failed"} {
		t.Run(kind, func(t *testing.T) {
			closed, queries := 0, 0
			api := wifiAPI{
				open: func() (uintptr, uint32) { return 7, 0 },
				close: func(h uintptr) uint32 {
					if h != 7 {
						t.Fatal("wrong handle")
					}
					closed++
					if kind == "close-failed" {
						return 6
					}
					return 0
				},
				allowed: func() bool { return kind != "permission" },
				interfaces: func(uintptr) ([]wlanInterface, uint32) {
					if kind == "no-interfaces" {
						return nil, 0
					}
					if kind == "enumeration-failed" {
						return nil, 5
					}
					state := uint32(1)
					if kind == "disconnected" {
						state = 4
					}
					return []wlanInterface{{guid: [16]byte{1}, state: state}}, 0
				},
				connection: func(uintptr, *[16]byte) (wlanConnection, uint32) {
					queries++
					if kind == "query-denied" {
						return wlanConnection{}, 5
					}
					c := wlanConnection{state: 1, association: wlanAssociation{ssid: wlanSSID{length: 3, value: [32]byte{'l', 'a', 'n'}}, bssid: [6]byte{2, 2, 3, 4, 5, 6}}}
					if kind == "invalid-ssid" {
						c.association.ssid.length = 33
					}
					if kind == "empty-ssid" {
						c.association.ssid.length = 0
					}
					if kind == "invalid-ap" {
						c.association.bssid[0] = 1
					}
					return c, 0
				},
			}
			s := readWiFi(api)
			if closed != 1 {
				t.Fatal("native owner leaked")
			}
			if kind == "permission" && (queries != 0 || !s.denied || s.observed) {
				t.Fatal("unconsented query invoked")
			}
			if kind == "connected" && (!s.observed || s.state != wifiConnected || len(s.associations) != 1) {
				t.Fatal("valid association lost")
			}
			if kind != "connected" && s.observed {
				t.Fatal("failed query claimed roaming observation", kind)
			}
			if kind == "no-interfaces" && s.state != wifiAbsent {
				t.Fatal("absence became unavailable")
			}
			if kind == "disconnected" && s.state != wifiDisconnected {
				t.Fatal("disconnection became unavailable")
			}
			if (kind == "enumeration-failed" || kind == "close-failed") && s.state != wifiUnknown {
				t.Fatal("failed inspection became observed absence")
			}
		})
	}
}

func TestLiveWiFiSamplingIsReadOnlyAndPromptFree(t *testing.T) {
	// CheckAccess, enum and (only if already allowed) current connection. No
	// scan, capability request, profile, radio or settings mutation is called.
	s := sampleWiFi()
	if s.state > wifiConnecting {
		t.Fatal("invalid native snapshot")
	}
	if s.observed && (s.state != wifiConnected || len(s.associations) == 0) {
		t.Fatal("observation without native association")
	}
	t.Logf("state=%d roamingObserved=%v accessUnavailable=%v", s.state, s.observed, s.denied)
}
