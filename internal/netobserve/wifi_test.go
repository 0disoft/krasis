package netobserve

import (
	"context"
	"testing"
	"time"
)

func TestWiFiRoamIsSeparateFromNetworkChangeAndUnknownGap(t *testing.T) {
	for _, kind := range []string{"same", "roam", "network", "blind-gap", "different-interface", "reconnect"} {
		t.Run(kind, func(t *testing.T) {
			var events []string
			m := newMonitor(context.Background(), func(code string) { events = append(events, code) })
			defer m.cancel()
			old := wifiSnapshot{state: wifiConnected, observed: true, associations: []wifiAssociation{{iface: [16]byte{1}, network: [32]byte{2}, ap: [32]byte{3}}}}
			m.observe(Snapshot{wifi: old}, time.Now())
			if len(m.restored) != 0 {
				t.Fatal("initial association revived a lease")
			}
			current := old
			current.associations = append([]wifiAssociation(nil), old.associations...)
			switch kind {
			case "roam":
				current.associations[0].ap[0]++
			case "network":
				current.associations[0].network[0]++
			case "blind-gap":
				m.observe(Snapshot{}, time.Now())
				current.associations[0].ap[0]++
			case "different-interface":
				current.associations[0].iface[0]++
				current.associations[0].ap[0]++
			case "reconnect":
				m.observe(Snapshot{wifi: wifiSnapshot{state: wifiDisconnected}}, time.Now())
			}
			m.observe(Snapshot{wifi: current}, time.Now())
			roam, network := false, false
			for _, event := range events {
				roam = roam || event == "network_wifi_ap_changed"
				network = network || event == "network_wifi_network_changed"
			}
			if roam != (kind == "roam") || network != (kind == "network") {
				t.Fatal("association change misclassified", kind, events)
			}
			wake := kind == "roam" || kind == "network" || kind == "reconnect"
			if (len(m.restored) == 1) != wake {
				t.Fatal("unexpected retry wake", kind, events)
			}
			if wake {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if err := m.Wait(ctx, time.Hour); err != nil || len(m.restored) != 0 {
					t.Fatal("restoration did not wake one serial retry", err)
				}
			}
		})
	}
}
