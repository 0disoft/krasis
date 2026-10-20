package netobserve

import (
	"context"
	"testing"
	"time"
)

func TestWiFiDiagnosticIDsAndObservationGaps(t *testing.T) {
	var reports []WiFiObservation
	m := newMonitor(context.Background(), nil)
	defer m.cancel()
	m.wifiReport = func(v WiFiObservation) { reports = append(reports, v) }
	s := Snapshot{wifi: wifiSnapshot{state: wifiConnected, observed: true, associations: []wifiAssociation{{iface: [16]byte{1}, network: [32]byte{2}, ap: [32]byte{3}}}}}
	base := time.Now()
	m.observe(s, base)
	m.observe(s, base.Add(time.Second))
	if len(reports) != 1 || !reports[0].Valid() || !reports[0].IdentifiersKnown {
		t.Fatal("initial anonymous observation missing")
	}
	s.wifi.associations = append([]wifiAssociation(nil), s.wifi.associations...)
	s.wifi.associations[0].ap[0]++
	m.observe(s, base.Add(2*time.Second))
	if len(reports) != 2 || reports[1].Reason != "ap_changed" || reports[1].Interface != reports[0].Interface || reports[1].AP == reports[0].AP || reports[1].SampleWindowMicros != time.Second.Microseconds() {
		t.Fatal("anonymous AP change lost")
	}
	m.observe(Snapshot{}, base.Add(3*time.Second))
	m.observe(s, base.Add(5*time.Second))
	if len(reports) != 4 || reports[3].Reason != "restored" || reports[3].ObservationGapMicros != (2*time.Second).Microseconds() {
		t.Fatal("unknown observation gap became an AP roam")
	}
	other := newMonitor(context.Background(), nil)
	defer other.cancel()
	var different WiFiObservation
	other.wifiReport = func(v WiFiObservation) { different = v }
	other.observe(s, base)
	if different.Interface == reports[0].Interface || different.AP == reports[1].AP {
		t.Fatal("diagnostic identity linked independent monitor lifetimes")
	}
	other.wifiKeyKnown = false
	other.previous = Snapshot{}
	other.started = false
	other.observe(s, base)
	if different.IdentifiersKnown || different.Interface != "" || different.AP != "" {
		t.Fatal("missing entropy exposed unsalted identity")
	}
}
