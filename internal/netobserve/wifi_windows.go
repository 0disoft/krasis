//go:build windows

package netobserve

import (
	"crypto/sha256"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const maxWiFiInterfaces = 32

type wlanInterface struct {
	guid        [16]byte
	description [256]uint16
	state       uint32
}
type wlanSSID struct {
	length uint32
	value  [32]byte
}
type wlanAssociation struct {
	ssid                                      wlanSSID
	bssType                                   uint32
	bssid                                     [6]byte
	padding                                   [2]byte
	phyType, phyIndex, signal, rxRate, txRate uint32
}
type wlanConnection struct {
	state, mode uint32
	profile     [256]uint16
	association wlanAssociation
	security    [4]uint32
}

var wlanDLL = syscall.NewLazyDLL("wlanapi.dll")
var wlanOpen = wlanDLL.NewProc("WlanOpenHandle")
var wlanClose = wlanDLL.NewProc("WlanCloseHandle")
var wlanEnum = wlanDLL.NewProc("WlanEnumInterfaces")
var wlanQuery = wlanDLL.NewProc("WlanQueryInterface")
var wlanFree = wlanDLL.NewProc("WlanFreeMemory")
var wifiDisabled atomic.Bool // A failed handle close must not accumulate another handle each poll.

type wifiAPI struct {
	open       func() (uintptr, uint32)
	close      func(uintptr) uint32
	interfaces func(uintptr) ([]wlanInterface, uint32)
	connection func(uintptr, *[16]byte) (wlanConnection, uint32)
	allowed    func() bool
}

func sampleWiFi() wifiSnapshot {
	if wifiDisabled.Load() {
		return wifiSnapshot{}
	}
	for _, p := range []*syscall.LazyProc{wlanOpen, wlanClose, wlanEnum, wlanQuery, wlanFree} {
		if p.Find() != nil {
			return wifiSnapshot{}
		}
	}
	return readWiFi(wifiAPI{
		open: func() (uintptr, uint32) {
			var h uintptr
			var version uint32
			r, _, _ := wlanOpen.Call(2, 0, uintptr(unsafe.Pointer(&version)), uintptr(unsafe.Pointer(&h)))
			return h, uint32(r)
		},
		close: func(h uintptr) uint32 {
			r, _, _ := wlanClose.Call(h, 0)
			if r != 0 {
				wifiDisabled.Store(true)
			}
			return uint32(r)
		},
		interfaces: func(h uintptr) ([]wlanInterface, uint32) {
			var data unsafe.Pointer
			r, _, _ := wlanEnum.Call(h, 0, uintptr(unsafe.Pointer(&data)))
			if data != nil {
				defer wlanFree.Call(uintptr(data))
			}
			if r != 0 {
				return nil, uint32(r)
			}
			if data == nil {
				return nil, 13
			}
			count := *(*uint32)(data)
			if count > maxWiFiInterfaces {
				return nil, 13
			}
			native := unsafe.Slice((*wlanInterface)(unsafe.Add(data, 8)), int(count))
			return append([]wlanInterface(nil), native...), 0
		},
		connection: func(h uintptr, guid *[16]byte) (wlanConnection, uint32) {
			var data unsafe.Pointer
			var size uint32
			r, _, _ := wlanQuery.Call(h, uintptr(unsafe.Pointer(guid)), 7, 0, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&data)), 0)
			if data != nil {
				defer wlanFree.Call(uintptr(data))
			}
			if r != 0 {
				return wlanConnection{}, uint32(r)
			}
			if data == nil || size < uint32(unsafe.Sizeof(wlanConnection{})) {
				return wlanConnection{}, 13
			}
			return *(*wlanConnection)(data), 0
		},
		allowed: wifiLocationAllowed,
	})
}

func readWiFi(api wifiAPI) (s wifiSnapshot) {
	h, code := api.open()
	if code != 0 || h == 0 {
		return s
	}
	defer func() {
		if api.close(h) != 0 {
			s = wifiSnapshot{}
		}
	}()
	interfaces, code := api.interfaces(h)
	if code != 0 || len(interfaces) > maxWiFiInterfaces {
		return s
	}
	if len(interfaces) == 0 {
		s.state = wifiAbsent
		return s
	}
	s.state = wifiDisconnected
	statesKnown := true
	var connected []wlanInterface
	for _, i := range interfaces {
		switch i.state {
		case 1:
			connected = append(connected, i)
		case 5, 6, 7:
			if s.state != wifiUnknown {
				s.state = wifiConnecting
			}
		case 0, 3, 4:
		default:
			s.state = wifiUnknown
			statesKnown = false
		}
	}
	if len(connected) == 0 {
		return s
	}
	s.state = wifiConnected
	if !statesKnown {
		return s
	}
	if !api.allowed() {
		s.denied = true
		return s
	}
	s.observed = true
	for _, i := range connected {
		c, code := api.connection(h, &i.guid)
		if code != 0 || c.state != 1 || c.association.ssid.length == 0 || c.association.ssid.length > 32 || c.association.bssid == ([6]byte{}) || c.association.bssid[0]&1 != 0 {
			s.observed = false
			s.associations = nil
			s.denied = code == 5
			return s
		}
		s.associations = append(s.associations, wifiAssociation{iface: i.guid, network: sha256.Sum256(c.association.ssid.value[:c.association.ssid.length]), ap: sha256.Sum256(c.association.bssid[:])})
	}
	return s
}
