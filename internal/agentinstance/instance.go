package agentinstance

import "errors"

var (
	ErrAlreadyRunning = errors.New("Krasis agent is already running in this user session")
	ErrUnsupported    = errors.New("Krasis agent single-instance ownership is unsupported on this platform")
)

const productionName = `Local\Krasis-Agent-v1`

type Lease struct {
	close func() error
}

func Acquire() (*Lease, error) { return acquire(productionName) }

func (lease *Lease) Close() error {
	if lease == nil || lease.close == nil {
		return nil
	}
	close := lease.close
	lease.close = nil
	return close()
}
