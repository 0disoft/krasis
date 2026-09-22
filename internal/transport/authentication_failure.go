package transport

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
)

var (
	ErrPeerTLSVersion       = errors.New("unsupported negotiated peer TLS version")
	ErrPeerALPN             = errors.New("unexpected negotiated peer ALPN")
	ErrPeerCertificateChain = errors.New("verified peer chain unavailable")
	ErrPeerRole             = errors.New("peer certificate role invalid")
)

func AuthenticationFailureCode(err error) string {
	var invalid x509.CertificateInvalidError
	var unknown x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var network net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return "tls_canceled"
	case errors.Is(err, context.DeadlineExceeded) || errors.As(err, &network) && network.Timeout():
		return "tls_timeout"
	case errors.As(err, &invalid):
		if invalid.Reason == x509.Expired {
			return "tls_certificate_time_invalid"
		}
		return "tls_certificate_invalid"
	case errors.As(err, &unknown):
		return "tls_untrusted"
	case errors.As(err, &hostname):
		return "tls_identity_mismatch"
	case errors.Is(err, ErrPeerTLSVersion):
		return "tls_version_rejected"
	case errors.Is(err, ErrPeerALPN):
		return "tls_alpn_rejected"
	case errors.Is(err, ErrPeerCertificateChain):
		return "tls_chain_unverified"
	case errors.Is(err, ErrPeerRole):
		return "tls_role_rejected"
	default:
		return "tls_handshake_failed"
	}
}
