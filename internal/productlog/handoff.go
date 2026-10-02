package productlog

import "github.com/0disoft/krasis/internal/handoff"

func init() { allowed["handoff_profile"] = true }

func (l *Logger) Handoff(s handoff.Summary) {
	if s.Valid() {
		l.enqueueRecord(record{Code: "handoff_profile", Handoff: &s})
	}
}
