package peerprobe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/transport"
)

const (
	SchemaVersion = "krasis.lab.peer-probe/v1"
	SessionCount  = 2
	probeLeaseID  = "peer-probe"
)

type Role string

const (
	RoleListen  Role = "listen"
	RoleConnect Role = "connect"
)

type Config struct {
	Role               Role
	DeviceAlias        string
	BuildRevision      string
	BuildModified      bool
	Address            string
	ExpectedServerName string
	CAPath             string
	CertificatePath    string
	PrivateKeyPath     string
	Timeout            time.Duration
	Ready              func(string)
}

type Result struct {
	SchemaVersion      string    `json:"schemaVersion"`
	Role               Role      `json:"role"`
	DeviceAlias        string    `json:"deviceAlias"`
	CollectedAt        time.Time `json:"collectedAt"`
	BuildRevision      string    `json:"buildRevision"`
	BuildModified      bool      `json:"buildModified"`
	Completed          bool      `json:"completed"`
	FailureCode        string    `json:"failureCode,omitempty"`
	Authenticated      bool      `json:"authenticated"`
	TLSVersion         string    `json:"tlsVersion,omitempty"`
	ALPN               string    `json:"alpn,omitempty"`
	SessionsCompleted  int       `json:"sessionsCompleted"`
	LeaseRoundTrips    int       `json:"leaseRoundTrips"`
	InputFramesChecked int       `json:"inputFramesChecked"`
	FreshIdentities    bool      `json:"freshIdentities"`
}

func Run(ctx context.Context, config Config) (result Result, err error) {
	result = Result{
		SchemaVersion: SchemaVersion,
		Role:          config.Role,
		DeviceAlias:   config.DeviceAlias,
		CollectedAt:   time.Now().UTC(),
		BuildRevision: config.BuildRevision,
		BuildModified: config.BuildModified,
	}
	if err := config.validate(); err != nil {
		return result, err
	}
	probeContext, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()

	identity, roots, err := loadCredentials(config)
	if err != nil {
		result.FailureCode = "credential_load_failed"
		return result, err
	}
	switch config.Role {
	case RoleListen:
		err = runListener(probeContext, config, identity, roots, &result)
	case RoleConnect:
		err = runConnector(probeContext, config, identity, roots, &result)
	}
	if err != nil {
		if result.FailureCode == "" {
			result.FailureCode = "peer_probe_failed"
		}
		return result, err
	}
	result.Completed = true
	result.FreshIdentities = result.SessionsCompleted == SessionCount
	return result, nil
}

func (config Config) validate() error {
	if config.Role != RoleListen && config.Role != RoleConnect {
		return errors.New("role must be listen or connect")
	}
	if config.Address == "" || config.CAPath == "" || config.CertificatePath == "" || config.PrivateKeyPath == "" {
		return errors.New("address, CA, certificate, and private key are required")
	}
	if config.Role == RoleConnect && config.ExpectedServerName == "" {
		return errors.New("expected server name is required in connect mode")
	}
	if config.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	return nil
}

func loadCredentials(config Config) (tls.Certificate, *x509.CertPool, error) {
	identity, err := tls.LoadX509KeyPair(config.CertificatePath, config.PrivateKeyPath)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("load identity: %w", err)
	}
	caPEM, err := os.ReadFile(config.CAPath)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("read CA: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return tls.Certificate{}, nil, errors.New("CA file contains no certificates")
	}
	return identity, roots, nil
}

func runListener(ctx context.Context, config Config, identity tls.Certificate, roots *x509.CertPool, result *Result) error {
	tlsConfig, err := transport.NewServerTLSConfig(identity, roots)
	if err != nil {
		result.FailureCode = "tls_config_failed"
		return err
	}
	listener, err := net.Listen("tcp", config.Address)
	if err != nil {
		result.FailureCode = "listen_failed"
		return err
	}
	defer listener.Close()
	if config.Ready != nil {
		config.Ready(listener.Addr().String())
	}
	stop := context.AfterFunc(ctx, func() { _ = listener.Close() })
	defer stop()

	seenSessions := make(map[string]struct{}, SessionCount)
	seenEpochs := make(map[string]struct{}, SessionCount)
	for range SessionCount {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			result.FailureCode = "accept_failed"
			return acceptErr
		}
		peerConnection, peerErr := transport.NewPeerConnection(ctx, tls.Server(connection, tlsConfig))
		if peerErr != nil {
			_ = connection.Close()
			result.FailureCode = "authentication_failed"
			return peerErr
		}
		applyTLSState(result)
		outbound, sessionErr := serveSession(ctx, peerConnection, result)
		_ = peerConnection.Close()
		if sessionErr != nil {
			result.FailureCode = "session_failed"
			return sessionErr
		}
		if _, duplicate := seenSessions[outbound.SessionID]; duplicate {
			result.FailureCode = "identity_reused"
			return errors.New("peer reused a session identifier")
		}
		if _, duplicate := seenEpochs[outbound.Epoch]; duplicate {
			result.FailureCode = "identity_reused"
			return errors.New("peer reused an epoch")
		}
		seenSessions[outbound.SessionID] = struct{}{}
		seenEpochs[outbound.Epoch] = struct{}{}
		result.SessionsCompleted++
	}
	return nil
}

func runConnector(ctx context.Context, config Config, identity tls.Certificate, roots *x509.CertPool, result *Result) error {
	tlsConfig, err := transport.NewClientTLSConfig(identity, roots, config.ExpectedServerName)
	if err != nil {
		result.FailureCode = "tls_config_failed"
		return err
	}
	seenSessions := make(map[string]struct{}, SessionCount)
	seenEpochs := make(map[string]struct{}, SessionCount)
	for range SessionCount {
		outbound, identityErr := peer.NewRandomOutboundIdentity()
		if identityErr != nil {
			result.FailureCode = "identity_generation_failed"
			return identityErr
		}
		if _, duplicate := seenSessions[outbound.SessionID]; duplicate {
			result.FailureCode = "identity_reused"
			return errors.New("random session identifier was reused")
		}
		if _, duplicate := seenEpochs[outbound.Epoch]; duplicate {
			result.FailureCode = "identity_reused"
			return errors.New("random epoch was reused")
		}
		seenSessions[outbound.SessionID] = struct{}{}
		seenEpochs[outbound.Epoch] = struct{}{}
		dialer := net.Dialer{Timeout: config.Timeout}
		connection, dialErr := dialer.DialContext(ctx, "tcp", config.Address)
		if dialErr != nil {
			result.FailureCode = "connect_failed"
			return dialErr
		}
		peerConnection, peerErr := transport.NewPeerConnection(ctx, tls.Client(connection, tlsConfig))
		if peerErr != nil {
			_ = connection.Close()
			result.FailureCode = "authentication_failed"
			return peerErr
		}
		applyTLSState(result)
		if sessionErr := requestSession(ctx, peerConnection, outbound, result); sessionErr != nil {
			_ = peerConnection.Close()
			result.FailureCode = "session_failed"
			return sessionErr
		}
		_ = peerConnection.Close()
		result.SessionsCompleted++
	}
	return nil
}

func serveSession(ctx context.Context, connection *transport.PeerConnection, result *Result) (peer.OutboundIdentity, error) {
	requestFrame, err := connection.ReadFrame()
	if err != nil {
		return peer.OutboundIdentity{}, err
	}
	if requestFrame.Kind != transport.MessageControl {
		return peer.OutboundIdentity{}, errors.New("expected lease request frame")
	}
	request, err := transport.DecodeControlPayload(requestFrame.Payload)
	if err != nil || request.Kind != transport.ControlLeaseRequest {
		return peer.OutboundIdentity{}, errors.New("invalid lease request")
	}
	grantPayload, err := transport.EncodeControlPayload(transport.ControlCommand{
		Kind: transport.ControlLeaseGrant, SessionID: request.SessionID, LeaseID: probeLeaseID, Epoch: request.Epoch,
	})
	if err != nil {
		return peer.OutboundIdentity{}, err
	}
	if err := connection.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageControl, Payload: grantPayload}); err != nil {
		return peer.OutboundIdentity{}, err
	}
	result.LeaseRoundTrips++

	inputFrame, err := connection.ReadFrame()
	if err != nil {
		return peer.OutboundIdentity{}, err
	}
	if inputFrame.Kind != transport.MessageInput {
		return peer.OutboundIdentity{}, errors.New("expected input frame")
	}
	envelope, action, err := transport.DecodeInputPayload(inputFrame.Payload)
	if err != nil || envelope.SessionID != request.SessionID || envelope.Epoch != request.Epoch || envelope.Sequence != 1 {
		return peer.OutboundIdentity{}, errors.New("input envelope does not match active probe session")
	}
	if action.Kind != control.InputPointerMove || action.DeltaX != 7 || action.DeltaY != -3 {
		return peer.OutboundIdentity{}, errors.New("unexpected probe input action")
	}
	result.InputFramesChecked++
	revokePayload, err := transport.EncodeControlPayload(transport.ControlCommand{
		Kind: transport.ControlLeaseRevoke, SessionID: request.SessionID, LeaseID: probeLeaseID, Epoch: request.Epoch,
	})
	if err != nil {
		return peer.OutboundIdentity{}, err
	}
	if err := connection.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageControl, Payload: revokePayload}); err != nil {
		return peer.OutboundIdentity{}, err
	}
	return peer.OutboundIdentity{SessionID: request.SessionID, Epoch: request.Epoch}, nil
}

func requestSession(ctx context.Context, connection *transport.PeerConnection, identity peer.OutboundIdentity, result *Result) error {
	requestPayload, err := transport.EncodeControlPayload(transport.ControlCommand{
		Kind: transport.ControlLeaseRequest, SessionID: identity.SessionID, Epoch: identity.Epoch, Edge: control.ScreenEdgeRight,
	})
	if err != nil {
		return err
	}
	if err := connection.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageControl, Payload: requestPayload}); err != nil {
		return err
	}
	grantFrame, err := connection.ReadFrame()
	if err != nil {
		return err
	}
	grant, err := decodeExactControl(grantFrame, transport.ControlLeaseGrant, identity, probeLeaseID)
	if err != nil {
		return err
	}
	result.LeaseRoundTrips++
	envelope := control.InputEnvelope{SessionID: identity.SessionID, Epoch: identity.Epoch, Sequence: 1}
	inputPayload, err := transport.EncodeInputPayload(envelope, control.InputAction{Kind: control.InputPointerMove, DeltaX: 7, DeltaY: -3})
	if err != nil {
		return err
	}
	if err := connection.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageInput, Payload: inputPayload}); err != nil {
		return err
	}
	result.InputFramesChecked++
	revokeFrame, err := connection.ReadFrame()
	if err != nil {
		return err
	}
	_, err = decodeExactControl(revokeFrame, transport.ControlLeaseRevoke, identity, grant.LeaseID)
	return err
}

func decodeExactControl(frame transport.Frame, kind transport.ControlCommandKind, identity peer.OutboundIdentity, leaseID string) (transport.ControlCommand, error) {
	if frame.Kind != transport.MessageControl {
		return transport.ControlCommand{}, errors.New("expected control frame")
	}
	command, err := transport.DecodeControlPayload(frame.Payload)
	if err != nil {
		return transport.ControlCommand{}, err
	}
	if command.Kind != kind || command.SessionID != identity.SessionID || command.Epoch != identity.Epoch || command.LeaseID != leaseID {
		return transport.ControlCommand{}, errors.New("control response does not match probe session")
	}
	return command, nil
}

func applyTLSState(result *Result) {
	result.Authenticated = true
	result.TLSVersion = "TLS1.3"
	result.ALPN = transport.ALPNProtocol
}
