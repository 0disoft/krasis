package netobserve

type wifiState uint8

const (
	wifiUnknown wifiState = iota
	wifiAbsent
	wifiDisconnected
	wifiConnected
	wifiConnecting
)

type wifiAssociation struct {
	iface       [16]byte
	network, ap [32]byte
}
type wifiSnapshot struct {
	state            wifiState
	observed, denied bool
	associations     []wifiAssociation // Comparison only; none of these identifiers are persisted.
}

func (m *Monitor) observeWiFi(s wifiSnapshot) bool {
	p := m.previous.wifi
	if !m.started || p.state != s.state {
		m.event([]string{"network_wifi_unknown", "network_wifi_absent", "network_wifi_disconnected", "network_wifi_connected", "network_wifi_connecting"}[s.state])
	}
	if !m.started || p.observed != s.observed || p.denied != s.denied {
		code := "network_roaming_unknown"
		if s.observed {
			code = "network_roaming_available"
		} else if s.denied {
			code = "network_roaming_access_unavailable"
		}
		m.event(code)
	}
	roamed, changed := false, false
	if s.observed && p.observed {
		for _, current := range s.associations {
			for _, old := range p.associations {
				if old.iface == current.iface {
					if old.network != current.network {
						changed = true
					} else if old.ap != current.ap {
						roamed = true
					}
				}
			}
		}
	}
	if roamed {
		m.event("network_wifi_ap_changed")
	}
	if changed {
		m.event("network_wifi_network_changed")
	}
	restored := s.state == wifiConnected && (m.lastWiFi == wifiDisconnected || m.lastWiFi == wifiConnecting)
	if s.state != wifiUnknown {
		m.lastWiFi = s.state
	}
	return s.state == wifiConnected && (restored || roamed || changed)
}
