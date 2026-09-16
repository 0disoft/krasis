package agent

import "errors"

// ErrRecoveryUnverified prevents automatic retries after a worker's cleanup
// failed. A logical LOCAL state alone does not prove native input was released.
var ErrRecoveryUnverified = errors.New("agent input recovery is unverified")
