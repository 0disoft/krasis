package residentagent

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/pairing"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/truststore"
)

const pairingALPN = "krasis-pairing/1"

type networkPairing struct {
	root       context.Context
	identity   *productidentity.Identity
	identities *productidentity.Store
	trust      *truststore.Store
	listen     func(string, string) (net.Listener, error)
	mu         sync.Mutex
	job        *pairJob
}

type pairJob struct {
	replacedFingerprint           string
	id, alias, code, confirmation string
	allow                         bool
	cancel                        context.CancelFunc
	done                          chan struct{}
	ready                         chan struct{}
	session                       *pairing.Session
	confirmed                     bool
	receipt                       chan pairing.Receipt
}

func (service *networkPairing) Active() bool {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.job == nil {
		return false
	}
	select {
	case <-service.job.done:
		return false
	default:
		return true
	}
}

func (service *networkPairing) Close(ctx context.Context) error {
	service.mu.Lock()
	job := service.job
	if job != nil {
		job.cancel()
	}
	service.mu.Unlock()
	if job == nil {
		return nil
	}
	select {
	case <-job.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (service *networkPairing) Handle(ctx context.Context, request localapi.OperationRequest) (localapi.OperationResult, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if ctx.Err() != nil || service.root.Err() != nil {
		return rejected("operation_cancelled"), nil
	}
	if request.PairingAction == "LISTEN" || request.PairingAction == "DIAL" {
		if request.AllowInboundControl == nil || (truststore.ConnectionSettings{Address: request.Address, Edge: "LEFT"}).Validate() != nil {
			return rejected("invalid_request"), nil
		}
		if service.job != nil {
			select {
			case <-service.job.done:
			default:
				return rejected("pairing_already_active"), nil
			}
		}
		role := transport.AuthenticatedPeerConnector
		if request.PairingAction == "LISTEN" {
			role = transport.AuthenticatedPeerListener
		}
		if request.Fingerprint != "" {
			document, err := service.trust.List()
			if err != nil {
				if errors.Is(err, truststore.ErrStoreNotFound) {
					return rejected("peer_not_found"), nil
				}
				return failed("trust_store_unavailable"), nil
			}
			found := false
			for _, peer := range document.Peers {
				if peer.Fingerprint != request.Fingerprint {
					continue
				}
				if peer.RevokedAt != nil {
					return rejected("peer_already_revoked"), nil
				}
				if peer.Role == role {
					return rejected("replacement_role_mismatch"), nil
				}
				found = true
			}
			if !found {
				return rejected("peer_not_found"), nil
			}
		}
		identity, err := service.identities.OpenTransport(role, time.Now())
		if err != nil {
			return failed("credential_unavailable"), nil
		}
		var listener net.Listener
		if role == transport.AuthenticatedPeerListener {
			listen := service.listen
			if listen == nil {
				listen = net.Listen
			}
			listener, err = listen("tcp", request.Address)
			if err != nil {
				return failed("pairing_listen_failed"), nil
			}
		}
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			if listener != nil {
				listener.Close()
			}
			return localapi.OperationResult{}, err
		}
		jobContext, cancel := context.WithTimeout(service.root, pairing.DefaultLifetime)
		job := &pairJob{id: hex.EncodeToString(id[:]), alias: request.PeerAlias, allow: *request.AllowInboundControl, code: "pairing_waiting", cancel: cancel, done: make(chan struct{}), ready: make(chan struct{}), receipt: make(chan pairing.Receipt, 1)}
		service.job = job
		job.replacedFingerprint = request.Fingerprint
		go service.run(jobContext, job, listener, request.Address, identity, role)
		return pairResult(job), nil
	}
	job := service.job
	if job == nil || job.id != request.PairingID {
		return rejected("pairing_not_found"), nil
	}
	if request.PairingAction == "STATUS" {
		return pairResult(job), nil
	}
	if job.code == "pairing_completed" || job.code == "pairing_cancelled" {
		return rejected("pairing_finished"), nil
	}
	select {
	case <-job.done:
		return rejected("pairing_finished"), nil
	default:
	}
	switch request.PairingAction {
	case "CANCEL":
		job.cancel()
		job.code = "pairing_cancelled"
		job.confirmation = ""
		return pairResult(job), nil
	case "CONFIRM":
		if job.session == nil {
			return rejected("pairing_not_ready"), nil
		}
		if job.confirmed {
			return rejected("confirmation_already_submitted"), nil
		}
		receipt, err := job.session.Confirm(request.ConfirmationCode, time.Now())
		if err != nil {
			job.cancel()
			job.code = "pairing_failed"
			job.confirmation = ""
			return rejected("confirmation_rejected"), nil
		}
		job.confirmed = true
		job.code = "confirmation_submitted"
		job.confirmation = ""
		job.receipt <- receipt
		return pairResult(job), nil
	default:
		return rejected("invalid_request"), nil
	}
}

func pairResult(job *pairJob) localapi.OperationResult {
	result := completed(job.code)
	if job.code == "pairing_failed" || job.code == "pairing_expired" {
		result.Status = "FAILED"
	}
	result.PairingID, result.Confirmation = job.id, job.confirmation
	return result
}

func (service *networkPairing) run(ctx context.Context, job *pairJob, listener net.Listener, address string, identity tls.Certificate, role transport.AuthenticatedPeerRole) {
	err := service.exchange(ctx, job, listener, address, identity, role)
	service.mu.Lock()
	if err == nil {
		job.code = "pairing_completed"
	} else if job.code != "pairing_cancelled" {
		job.code = "pairing_failed"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			job.code = "pairing_expired"
		}
	}
	job.confirmation = ""
	job.session = nil
	job.cancel()
	close(job.done)
	service.mu.Unlock()
}

func (service *networkPairing) exchange(ctx context.Context, job *pairJob, listener net.Listener, address string, identity tls.Certificate, role transport.AuthenticatedPeerRole) error {
	var raw net.Conn
	var err error
	if listener != nil {
		defer listener.Close()
		stop := context.AfterFunc(ctx, func() { listener.Close() })
		defer stop()
		raw, err = listener.Accept()
	} else {
		raw, err = (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return err
	}
	defer raw.Close()
	stop := context.AfterFunc(ctx, func() { raw.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err := raw.SetDeadline(deadline); err != nil {
		return err
	}
	config := &tls.Config{Certificates: []tls.Certificate{identity}, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, NextProtos: []string{pairingALPN}, SessionTicketsDisabled: true}
	var connection *tls.Conn
	if role == transport.AuthenticatedPeerListener {
		config.ClientAuth = tls.RequireAnyClientCert
		connection = tls.Server(raw, config)
	} else {
		// Bootstrap channel only: the transcript's user-confirmed SAS authenticates
		// the exact TLS certificate below. This config never carries input frames.
		config.InsecureSkipVerify = true
		connection = tls.Client(raw, config)
	}
	handshake, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = connection.HandshakeContext(handshake)
	cancel()
	if err != nil {
		return err
	}
	state := connection.ConnectionState()
	if state.NegotiatedProtocol != pairingALPN || len(state.PeerCertificates) != 1 {
		return errors.New("invalid pairing TLS peer")
	}
	leaf := state.PeerCertificates[0]
	peer, err := transport.AuthenticatedPeerFromCertificate(leaf)
	if err != nil || peer.Role == role || leaf.IsCA || time.Now().Before(leaf.NotBefore) || !time.Now().Before(leaf.NotAfter) || leaf.KeyUsage != x509.KeyUsageDigitalSignature {
		return errors.New("invalid pairing certificate")
	}
	if err := leaf.CheckSignature(leaf.SignatureAlgorithm, leaf.RawTBSCertificate, leaf.Signature); err != nil {
		return err
	}
	local, err := transport.AuthenticatedPeerFromCertificate(identity.Leaf)
	if err != nil {
		return err
	}
	var session *pairing.Session
	if listener != nil {
		offer, err := pairing.CreateOffer(service.identity, local, time.Now())
		if err != nil {
			return err
		}
		if err := writePairMessage(connection, offer); err != nil {
			return err
		}
		var response pairing.Response
		if err := readPairMessage(connection, &response); err != nil {
			return err
		}
		if response.Responder.TransportPeer != peer {
			return errors.New("pairing response certificate mismatch")
		}
		session, err = pairing.Complete(offer, response, service.identity, time.Now())
		if err != nil {
			return err
		}
	} else {
		var offer pairing.Offer
		if err := readPairMessage(connection, &offer); err != nil {
			return err
		}
		if offer.Initiator.TransportPeer != peer {
			return errors.New("pairing offer certificate mismatch")
		}
		var response pairing.Response
		session, response, err = pairing.Accept(offer, service.identity, local, time.Now())
		if err != nil {
			return err
		}
		if err := writePairMessage(connection, response); err != nil {
			return err
		}
	}
	service.mu.Lock()
	if ctx.Err() != nil {
		service.mu.Unlock()
		return ctx.Err()
	}
	job.session, job.confirmation, job.code = session, session.ConfirmationCode(), "confirmation_required"
	close(job.ready)
	service.mu.Unlock()
	var receipt pairing.Receipt
	select {
	case receipt = <-job.receipt:
	case <-ctx.Done():
		return ctx.Err()
	}
	var remote pairing.Receipt
	if listener != nil {
		if err := writePairMessage(connection, receipt); err != nil {
			return err
		}
		if err := readPairMessage(connection, &remote); err != nil {
			return err
		}
	} else {
		if err := readPairMessage(connection, &remote); err != nil {
			return err
		}
		if err := writePairMessage(connection, receipt); err != nil {
			return err
		}
	}
	// Cancellation and final authorization are serialized with local CANCEL.
	service.mu.Lock()
	defer service.mu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if job.replacedFingerprint != "" {
		_, _, err = session.FinalizeReplacementWithCertificate(remote, leaf.Raw, service.trust, job.replacedFingerprint, job.alias, job.allow, time.Now())
	} else {
		_, err = session.FinalizeWithCertificate(remote, leaf.Raw, service.trust, job.alias, job.allow, time.Now())
	}
	if err == nil {
		job.code = "pairing_completed"
	}
	return err
}
