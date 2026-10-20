package productlog

import "github.com/0disoft/krasis/internal/netobserve"

func init() {
	allowed["network_wifi_observation"] = true
	for _, code := range netobserve.Codes() {
		allowed[code] = true
	}
}

type WiFiSummary = netobserve.WiFiObservation

func (l *Logger) WiFi(v netobserve.WiFiObservation) {
	if v.Valid() {
		l.enqueueRecord(record{Code: "network_wifi_observation", WiFi: &v})
	}
}
