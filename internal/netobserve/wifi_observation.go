package netobserve

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// IDs are HMAC pseudonyms scoped to this monitor, never native GUID/BSSID or
// SSID hashes. Durations describe observation windows, not transport outages.
type WiFiObservation struct {
	State                string `json:"state"`
	Reason               string `json:"reason"`
	IdentifiersKnown     bool   `json:"identifiersKnown"`
	Interface            string `json:"interface,omitempty"`
	AP                   string `json:"ap,omitempty"`
	SampleWindowMicros   int64  `json:"sampleWindowMicros"`
	ObservationGapMicros int64  `json:"observationGapMicros"`
}

func (v WiFiObservation) Valid() bool {
	if v.SampleWindowMicros < 0 || v.ObservationGapMicros < 0 {
		return false
	}
	switch v.State {
	case "unknown", "absent", "disconnected", "connected", "connecting":
	default:
		return false
	}
	switch v.Reason {
	case "observed", "ap_changed", "network_changed", "restored":
		if v.State != "connected" {
			return false
		}
	case "unavailable":
	default:
		return false
	}
	if !v.IdentifiersKnown {
		return v.Interface == "" && v.AP == ""
	}
	validID := func(s string) bool {
		if len(s) != 16 {
			return false
		}
		for _, c := range s {
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				return false
			}
		}
		return true
	}
	return v.State == "connected" && validID(v.Interface) && validID(v.AP)
}

func (m *Monitor) wifiObservation(s wifiSnapshot, a *wifiAssociation, reason string, at time.Time) {
	if m.wifiReport == nil {
		return
	}
	v := WiFiObservation{State: []string{"unknown", "absent", "disconnected", "connected", "connecting"}[s.state], Reason: reason}
	if !m.wifiLastAt.IsZero() {
		v.SampleWindowMicros = max(0, at.Sub(m.wifiLastAt).Microseconds())
	}
	if !m.wifiGapAt.IsZero() {
		v.ObservationGapMicros = max(0, at.Sub(m.wifiGapAt).Microseconds())
	}
	if a != nil && m.wifiKeyKnown {
		id := func(domain string, data []byte) string {
			h := hmac.New(sha256.New, m.wifiKey[:])
			h.Write([]byte(domain))
			h.Write(data)
			return hex.EncodeToString(h.Sum(nil)[:8])
		}
		v.IdentifiersKnown = true
		v.Interface = id("interface", a.iface[:])
		v.AP = id("ap", a.ap[:])
	}
	if v.Valid() {
		m.wifiReport(v)
	}
}
