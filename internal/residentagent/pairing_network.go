package residentagent

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/pairing"
	"github.com/0disoft/krasis/internal/pinauth"
	"github.com/0disoft/krasis/internal/pindiscovery"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/truststore"
)

const pairingALPN = "krasis-pairing/1"

const pairingJobLifetime = 2 * time.Minute
const pairingRetryDelay = 30 * time.Second

type networkPairing struct {
	root       context.Context
	identity   *productidentity.Identity
	identities *productidentity.Store
	trust      *truststore.Store
	listen     func(string, string) (net.Listener, error)
	advertise  func(context.Context, string, int) (io.Closer, error)
	mu         sync.Mutex
	job        *pairJob
	nextStart  time.Time
}

type pairJob struct {
	pinMode                       bool
	pin, invitation               string
	expiresAt                     time.Time
	replacedFingerprint           string
	id, alias, code, confirmation string
	allow                         bool
	cancel                        context.CancelFunc
	done                          chan struct{}
	ready                         chan struct{}
	session                       *pairing.Session
	confirmed                     bool
	commitAttempted               bool
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
	pinMode := request.PairingAction == "PIN_LISTEN" || request.PairingAction == "PIN_DIAL"
	if pinMode && request.PairingAction == "PIN_DIAL" && (!pinauth.Valid(request.ConfirmationCode) || len(request.PairingID) != 32) {
		return rejected("invalid_request"), nil
	}
	if request.PairingAction == "LISTEN" || request.PairingAction == "DIAL" || pinMode {
		if time.Now().Before(service.nextStart) {
			return rejected("pairing_retry_later"), nil
		}
		if request.AllowInboundControl == nil || (request.PairingAction != "PIN_LISTEN" && (truststore.ConnectionSettings{Address: request.Address, Edge: "LEFT"}).Validate() != nil) {
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
		if request.PairingAction == "LISTEN" || request.PairingAction == "PIN_LISTEN" {
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
			listenAddress := request.Address
			if request.PairingAction == "PIN_LISTEN" {
				listenAddress = "0.0.0.0:45871"
			}
			listener, err = listen("tcp4", listenAddress)
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
		expiresAt := time.Now().Add(pairingJobLifetime)
		jobContext, cancel := context.WithDeadline(service.root, expiresAt)
		job := &pairJob{id: hex.EncodeToString(id[:]), alias: request.PeerAlias, allow: *request.AllowInboundControl, code: "pairing_waiting", cancel: cancel, done: make(chan struct{}), ready: make(chan struct{}), receipt: make(chan pairing.Receipt, 1)}
		job.expiresAt = expiresAt
		job.pinMode = pinMode
		if pinMode {
			job.alias = request.PeerAlias + "-" + job.id[:8]
			job.pin = request.ConfirmationCode
			job.invitation = request.PairingID
			if role == transport.AuthenticatedPeerListener {
				job.pin, err = pinauth.Generate()
				if err != nil {
					cancel()
					listener.Close()
					return failed("pin_unavailable"), nil
				}
				job.invitation = job.id
				job.confirmation = job.pin
				_, portText, _ := net.SplitHostPort(listener.Addr().String())
				port, _ := strconv.Atoi(portText)
				advertise := service.advertise
				if advertise == nil {
					advertise = pindiscovery.Advertise
				}
				advertisement, err := advertise(jobContext, job.id, port)
				if err != nil {
					cancel()
					listener.Close()
					return failed("discovery_unavailable"), nil
				}
				context.AfterFunc(jobContext, func() { advertisement.Close() })
			}
			job.code = "pin_waiting"
		}
		service.job = job
		job.replacedFingerprint = request.Fingerprint
		go service.run(jobContext, job, listener, request.Address, identity, role)
		return pairResult(job), nil
	}
	job := service.job
	if job == nil || job.id != request.PairingID {
		return rejected("pairing_not_found"), nil
	}
	if !job.expiresAt.IsZero() && !time.Now().Before(job.expiresAt) {
		select {
		case <-job.done:
		default:
			if job.code != "pairing_completed" && job.code != "pairing_cancelled" {
				job.cancel()
				job.code = "pairing_expired"
				job.confirmation = ""
			}
		}
	}
	if request.PairingAction == "STATUS" {
		return pairResult(job), nil
	}
	if job.code == "pairing_completed" || job.code == "pairing_cancelled" || job.code == "pairing_expired" {
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
		if job.pinMode {
			return rejected("invalid_request"), nil
		}
		if job.session == nil {
			return rejected("pairing_not_ready"), nil
		}
		if job.confirmed {
			return rejected("confirmation_already_submitted"), nil
		}
		receipt, err := job.session.Confirm(request.ConfirmationCode, time.Now())
		if err != nil {
			service.nextStart = time.Now().Add(pairingRetryDelay)
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
	if job.code == "pairing_failed" || job.code == "pairing_expired" || job.code == "pairing_commit_uncertain" {
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
	} else if job.code != "pairing_cancelled" && job.code != "pairing_expired" {
		job.code = "pairing_failed"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			job.code = "pairing_expired"
		}
		// A failed finalizer may have persisted state before returning an error.
		// Do not report that no trust exists or infer the remote commit outcome.
		if job.commitAttempted {
			job.code = "pairing_commit_uncertain"
		}
	}
	job.confirmation = ""
	job.pin = ""
	if job.pinMode && err != nil {
		service.nextStart = time.Now().Add(pairingRetryDelay)
	}
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
	alpn := pairingALPN
	if job.pinMode {
		alpn = "krasis-pin/1"
	}
	config := &tls.Config{Certificates: []tls.Certificate{identity}, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, NextProtos: []string{alpn}, SessionTicketsDisabled: true}
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
	if state.NegotiatedProtocol != alpn || len(state.PeerCertificates) != 1 {
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
	if job.pinMode {
		remoteHost, _, _ := net.SplitHostPort(raw.RemoteAddr().String())
		ip := net.ParseIP(remoteHost)
		if ip == nil || (!ip.IsPrivate() && !ip.IsLoopback()) {
			return errors.New("PIN peer is not local")
		}
		binding, err := state.ExportKeyingMaterial("EXPORTER-Krasis-PIN-v1", []byte(job.invitation), 32)
		if err != nil {
			return err
		}
		if err = pinauth.Exchange(connection, job.pin, binding, listener != nil); err != nil {
			return err
		}
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
	if !time.Now().Before(job.expiresAt) {
		service.mu.Unlock()
		return context.DeadlineExceeded
	}
	job.session, job.confirmation, job.code = session, session.ConfirmationCode(), "confirmation_required"
	if job.pinMode {
		receipt, err := session.Confirm(session.ConfirmationCode(), time.Now())
		if err != nil {
			service.mu.Unlock()
			return err
		}
		job.confirmation = ""
		job.code = "pairing_registering"
		job.confirmed = true
		job.receipt <- receipt
	}
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
	if !time.Now().Before(job.expiresAt) {
		return context.DeadlineExceeded
	}
	job.commitAttempted = true
	if job.replacedFingerprint != "" {
		_, _, err = session.FinalizeReplacementWithCertificate(remote, leaf.Raw, service.trust, job.replacedFingerprint, job.alias, job.allow, time.Now())
	} else {
		_, err = session.FinalizeWithCertificate(remote, leaf.Raw, service.trust, job.alias, job.allow, time.Now())
	}
	if err == nil {
		if job.pinMode {
			address := raw.RemoteAddr()
			edge := "LEFT"
			if listener != nil {
				address = raw.LocalAddr()
				edge = "RIGHT"
			}
			host, _, _ := net.SplitHostPort(address.String())
			_, err = service.trust.SetConnection(job.alias, truststore.ConnectionSettings{Address: net.JoinHostPort(host, "45872"), Edge: edge})
			if err != nil {
				return err
			}
		}
		if !job.pinMode {
			job.code = "pairing_completed"
		}
	}
	return err
}
