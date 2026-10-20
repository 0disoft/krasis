package productlog

import "github.com/0disoft/krasis/internal/netobserve"

func init() {
	allowed["network_wifi_observation"] = true
	allowed["network_dial_observation"] = true
	for _, code := range netobserve.Codes() {
		allowed[code] = true
	}
}

type WiFiSummary = netobserve.WiFiObservation
type DialSummary = netobserve.DialObservation

func (l *Logger) Dial(v DialSummary) {
	if v.Valid() {
		l.enqueueRecord(record{Code: "network_dial_observation", Dial: &v})
	}
}

func (l *Logger) WiFi(v netobserve.WiFiObservation) {
	if v.Valid() {
		l.enqueueRecord(record{Code: "network_wifi_observation", WiFi: &v})
	}
}
