package peerupdate

import (
	"net/http"
	"time"
)

const auxiliaryIdleTimeout = time.Second

// Reuse immediate status, app-open and diagnostic exchanges. The idle
// bound matches the existing server policy; bulk transfer retains its own
// transport. TLS pinning and each handler's fresh trust check are unchanged.
func newControlTransport(base *http.Transport) *http.Transport {
	t := base.Clone()
	t.DisableKeepAlives = false
	t.MaxIdleConns = 1
	t.MaxIdleConnsPerHost = 1
	t.IdleConnTimeout = auxiliaryIdleTimeout
	return t
}
