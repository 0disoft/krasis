// Package keyboardstate contains display-only keyboard mode observations.
// Peer observations never grant input authority or trigger automatic injection.
package keyboardstate

type State struct {
	Caps   string `json:"caps"`
	Num    string `json:"num"`
	Hangul string `json:"hangul"`
}

func Unknown() State             { return State{Caps: "UNKNOWN", Num: "UNKNOWN", Hangul: "UNKNOWN"} }
func validMode(s string) bool    { return s == "ON" || s == "OFF" || s == "UNKNOWN" }
func (s State) Valid() bool      { return validMode(s.Caps) && validMode(s.Num) && validMode(s.Hangul) }
func (s State) LocksKnown() bool { return s.Valid() && s.Caps != "UNKNOWN" && s.Num != "UNKNOWN" }
