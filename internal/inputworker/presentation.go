package inputworker

import "time"

const presentationWindow = 60 * time.Second
const presentationAdmissionWindow = time.Second

// One presentation can consume one clean return, on the same authenticated
// kernel agent PID. A preparation hint cannot renew or resurrect this receipt.
type presentationReturn struct {
	lease   Lease
	pid     uint32
	started uint64
	at      time.Time
}

func (r *presentationReturn) take(lease Lease, pid uint32, started uint64, now time.Time) bool {
	valid := lease.Presentation && !r.lease.Presentation && r.pid != 0 && r.pid == pid && r.started != 0 && r.started == started &&
		lease.Identity == r.lease.Identity && !r.at.IsZero() && !now.Before(r.at) && now.Sub(r.at) <= presentationAdmissionWindow &&
		lease.ExpiresAt.After(now) && !lease.ExpiresAt.After(now.Add(presentationWindow)) && !lease.ExpiresAt.After(r.lease.ExpiresAt)
	if valid {
		*r = presentationReturn{}
	}
	return valid
}
