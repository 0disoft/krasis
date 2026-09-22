package productlog

type RetrySummary struct {
	Policy        string `json:"policy"`
	Unlimited     bool   `json:"unlimited"`
	Attempt       uint32 `json:"attempt"`
	NextMillis    int64  `json:"nextMillis"`
	InitialMillis int64  `json:"initialMillis"`
	MaximumMillis int64  `json:"maximumMillis"`
	StableMillis  int64  `json:"stableMillis"`
	StableReset   bool   `json:"stableReset"`
	JitterPercent int    `json:"jitterPercent"`
}

func init() { allowed["retry_policy"] = true }
func (s RetrySummary) valid() bool {
	return s.Policy == "agent-exponential-v1" && s.Unlimited && s.InitialMillis > 0 && s.MaximumMillis >= s.InitialMillis && s.MaximumMillis <= 3600000 && s.StableMillis > 0 && s.StableMillis <= 86400000 && s.NextMillis >= 0 && s.NextMillis <= s.MaximumMillis && (s.JitterPercent == 0 || s.JitterPercent == 20)
}
func (l *Logger) Retry(s RetrySummary) {
	if s.valid() {
		l.enqueueRecord(record{Code: "retry_policy", Retry: &s})
	}
}
