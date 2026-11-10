// Package x11desktop implements the unprivileged X11 desktop backend. It never
// grants a peer authority; the existing authenticated lease owns every call.
package x11desktop

import (
	"fmt"
	"strings"
)

// Physical XKB names, rather than keysyms or a fixed evdev offset, keep the
// existing Windows set-1 wire codes independent of the user's keyboard layout.
var physicalKeys = map[string]uint32{
	"ESC": 1, "BKSP": 14, "TAB": 15, "RTRN": 28, "LCTL": 29,
	"LFSH": 42, "BKSL": 43, "RTSH": 54, "KPMU": 55, "LALT": 56,
	"SPCE": 57, "CAPS": 58, "NMLK": 69, "SCLK": 70, "KPSU": 74,
	"KPAD": 78, "LSGT": 86, "KPEN": 0x1001c, "RCTL": 0x1001d,
	"KPDV": 0x10035, "PRSC": 0x10037, "RALT": 0x10038,
	"HOME": 0x10047, "UP": 0x10048, "PGUP": 0x10049,
	"LEFT": 0x1004b, "RGHT": 0x1004d, "END": 0x1004f,
	"DOWN": 0x10050, "PGDN": 0x10051, "INS": 0x10052, "DELE": 0x10053,
	"LWIN": 0x1005b, "RWIN": 0x1005c, "MENU": 0x1005d,
	"TLDE": 41, "AB11": 0x73, "HENK": 0x79, "MUHE": 0x7b,
	"HKTG": 0x70, "HIRA": 0x77, "KATA": 0x78, "AE13": 0x7d,
	"HNGL": 0xf2, "HJCV": 0xf1,
}

func scanForName(name string) (uint32, bool) {
	name = strings.TrimRight(name, "\x00 ")
	if code, ok := physicalKeys[name]; ok {
		return code, true
	}
	rows := []struct {
		prefix            string
		first, last, scan int
	}{
		{"AE", 1, 12, 2}, {"AD", 1, 12, 16}, {"AC", 1, 11, 30},
		{"AB", 1, 10, 44}, {"FK", 1, 10, 59}, {"FK", 11, 12, 87},
	}
	for _, row := range rows {
		for n := row.first; n <= row.last; n++ {
			if name == fmt.Sprintf("%s%02d", row.prefix, n) {
				return uint32(row.scan + n - row.first), true
			}
		}
	}
	keypad := map[string]uint32{"KP7": 71, "KP8": 72, "KP9": 73, "KP4": 75, "KP5": 76, "KP6": 77, "KP1": 79, "KP2": 80, "KP3": 81, "KP0": 82, "KPDL": 83}
	code, ok := keypad[name]
	return code, ok
}

func keyMaps(first int, names []byte) (map[byte]uint32, map[uint32]byte, error) {
	if first < 8 || len(names)%4 != 0 || first+len(names)/4 > 256 {
		return nil, nil, fmt.Errorf("invalid XKB key-name bounds")
	}
	forward, reverse := make(map[byte]uint32), make(map[uint32]byte)
	for i := 0; i < len(names)/4; i++ {
		code, ok := scanForName(string(names[i*4 : i*4+4]))
		if !ok {
			continue
		}
		if _, duplicate := reverse[code]; duplicate {
			return nil, nil, fmt.Errorf("ambiguous XKB physical key")
		}
		forward[byte(first+i)], reverse[code] = code, byte(first+i)
	}
	if _, ok := reverse[1]; !ok {
		return nil, nil, fmt.Errorf("XKB keyboard has no Escape key")
	}
	return forward, reverse, nil
}
