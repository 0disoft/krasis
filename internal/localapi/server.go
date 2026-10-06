package localapi

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/0disoft/krasis/internal/truststore"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	StatusSchemaVersion       = "krasis.agent.status/v1"
	OperationSchemaVersion    = "krasis.agent.operation/v1"
	ResultSchemaVersion       = "krasis.agent.operation-result/v1"
	DiagnosticsSchemaVersion  = "krasis.agent.diagnostics/v1"
	TrustSchemaVersion        = "krasis.agent.trust/v1"
	TopologySchemaVersion     = "krasis.agent.topology/v1"
	DefaultShutdownTimeout    = 5 * time.Second
	DefaultSessionStopTimeout = 5 * time.Second
	requestReadTimeout        = 5 * time.Second
	// The write deadline includes body reading and the bounded operation itself.
	responseWriteTimeout = requestReadTimeout + DefaultSessionStopTimeout + time.Second
	clientRequestTimeout = responseWriteTimeout + time.Second
	maxRequestBytes      = 64 << 10
)

type Status struct {
	BoundaryBlocked string `json:"boundaryBlocked,omitempty"`
	// InputBackend reports the selected runner path, not input authority or lease readiness.
	InputBackend       string   `json:"inputBackend,omitempty"`
	SchemaVersion      string   `json:"schemaVersion"`
	State              string   `json:"state"`
	PeerAlias          string   `json:"peerAlias,omitempty"`
	StopReason         string   `json:"stopReason,omitempty"`
	Actions            []string `json:"actions"`
	DeviceFingerprint  string   `json:"deviceFingerprint"`
	IdentityGeneration uint64   `json:"identityGeneration"`
}

type OperationRequest struct {
	SchemaVersion       string `json:"schemaVersion"`
	PeerAlias           string `json:"peerAlias,omitempty"`
	PairingAction       string `json:"pairingAction,omitempty"`
	PairingID           string `json:"pairingId,omitempty"`
	ConfirmationCode    string `json:"confirmationCode,omitempty"`
	AllowInboundControl *bool  `json:"allowInboundControl,omitempty"`
	Fingerprint         string `json:"fingerprint,omitempty"`
	Address             string `json:"address,omitempty"`
}

type OperationResult struct {
	SchemaVersion string `json:"schemaVersion"`
	Status        string `json:"status"`
	Code          string `json:"code"`
	PairingID     string `json:"pairingId,omitempty"`
	Confirmation  string `json:"confirmationCode,omitempty"`
}

type Diagnostics struct {
	SchemaVersion string            `json:"schemaVersion"`
	State         string            `json:"state"`
	Counters      map[string]uint64 `json:"counters"`
}

type TrustedPeer struct {
	Fingerprint         string `json:"fingerprint"`
	Alias               string `json:"alias"`
	Role                string `json:"role"`
	AllowInboundControl bool   `json:"allowInboundControl"`
	Revoked             bool   `json:"revoked"`
}

type Trust struct {
	SchemaVersion string        `json:"schemaVersion"`
	Revision      uint64        `json:"revision"`
	Peers         []TrustedPeer `json:"peers"`
}

type TopologyPeer struct {
	Alias               string `json:"alias"`
	AllowInboundControl bool   `json:"allowInboundControl"`
	Connected           bool   `json:"connected"`
}

type Topology struct {
	SchemaVersion string         `json:"schemaVersion"`
	State         string         `json:"state"`
	Layout        string         `json:"layout"`
	Peers         []TopologyPeer `json:"peers"`
}

type Operations interface {
	Status(context.Context) (Status, error)
	Connect(context.Context, OperationRequest) (OperationResult, error)
	Disconnect(context.Context, OperationRequest) (OperationResult, error)
	Pair(context.Context, OperationRequest) (OperationResult, error)
	Diagnostics(context.Context) (Diagnostics, error)
	Trust(context.Context) (Trust, error)
	RevokeTrust(context.Context, OperationRequest) (OperationResult, error)
	Topology(context.Context) (Topology, error)
	SafeStop(context.Context) (OperationResult, error)
}

type Server struct {
	listener    net.Listener
	http        *http.Server
	credentials *CredentialSession
	closeOnce   sync.Once
	closeErr    error
}

func Listen(controlDirectory string, operations Operations) (*Server, error) {
	if operations == nil {
		return nil, errors.New("local API operations are required")
	}
	runtimeInfo, err := newRuntimeInfo()
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	credentials, err := PublishCredentials(controlDirectory, listener.Addr().String())
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	handler := &handler{token: credentials.Token(), operations: operations, runtime: runtimeInfo}
	return &Server{
		listener:    listener,
		credentials: credentials,
		http: &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: 2 * time.Second,
			ReadTimeout:       requestReadTimeout,
			WriteTimeout:      responseWriteTimeout,
			IdleTimeout:       30 * time.Second,
		},
	}, nil
}

func (server *Server) Run(ctx context.Context) error {
	if server == nil || server.listener == nil || server.http == nil || ctx == nil {
		return errors.New("local API server and context are required")
	}
	result := make(chan error, 1)
	go func() {
		err := server.http.Serve(server.listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		result <- err
	}()
	select {
	case err := <-result:
		shutdown, cancel := context.WithTimeout(context.Background(), DefaultShutdownTimeout)
		defer cancel()
		closeErr := server.Close(shutdown)
		return errors.Join(err, closeErr)
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), DefaultShutdownTimeout)
		defer cancel()
		closeErr := server.Close(shutdown)
		serveErr := <-result
		return errors.Join(closeErr, serveErr)
	}
}

func (server *Server) Close(ctx context.Context) error {
	if server == nil {
		return nil
	}
	server.closeOnce.Do(func() {
		if ctx == nil {
			ctx = context.Background()
		}
		if err := server.http.Shutdown(ctx); err != nil {
			server.closeErr = errors.Join(server.closeErr, err, server.http.Close())
		}
		server.closeErr = errors.Join(server.closeErr, server.credentials.Close())
	})
	return server.closeErr
}

type handler struct {
	runtime    RuntimeInfo
	token      string
	operations Operations
}

func (handler *handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Content-Type", "application/json")
	if !handler.authenticated(request.Header.Get("Authorization")) {
		writeResult(response, http.StatusUnauthorized, OperationResult{SchemaVersion: ResultSchemaVersion, Status: "REJECTED", Code: "unauthenticated"})
		return
	}
	switch request.URL.Path {
	case "/v1/app-open":
		handler.appOpen(response, request)
	case "/v1/ui-ready":
		handler.uiReady(response, request)
	case "/v1/clipboard":
		handler.clipboard(response, request)
	case "/v1/diagnostic-transfer":
		handler.diagnosticTransfer(response, request)
	case "/v1/diagnostic-collection":
		handler.diagnosticCollection(response, request)
	case "/v1/runtime":
		if request.Method != http.MethodGet {
			writeMethodNotAllowed(response)
			return
		}
		writeJSON(response, http.StatusOK, handler.runtime)
	case "/v1/upgrade-stop":
		handler.upgradeStop(response, request)
	case "/v1/release-claim":
		handler.releaseClaim(response, request)
	case "/v1/update":
		handler.update(response, request)
	case "/v1/credential":
		handler.credential(response, request)
	case "/v1/connection":
		handler.connection(response, request)
	case "/v1/status":
		if request.Method != http.MethodGet {
			writeMethodNotAllowed(response)
			return
		}
		value, err := handler.operations.Status(request.Context())
		if err != nil {
			writeInternalFailure(response)
			return
		}
		writeJSON(response, http.StatusOK, value)
	case "/v1/connect":
		handler.operation(response, request, "connect", handler.operations.Connect)
	case "/v1/disconnect":
		handler.operation(response, request, "disconnect", handler.operations.Disconnect)
	case "/v1/pair":
		handler.operation(response, request, "pair", handler.operations.Pair)
	case "/v1/diagnostics":
		if request.Method != http.MethodGet {
			writeMethodNotAllowed(response)
			return
		}
		value, err := handler.operations.Diagnostics(request.Context())
		if err != nil {
			writeInternalFailure(response)
			return
		}
		writeJSON(response, http.StatusOK, value)
	case "/v1/trust":
		if request.Method != http.MethodGet {
			writeMethodNotAllowed(response)
			return
		}
		value, err := handler.operations.Trust(request.Context())
		if err != nil {
			writeInternalFailure(response)
			return
		}
		writeJSON(response, http.StatusOK, value)
	case "/v1/trust/revoke":
		handler.operation(response, request, "trust-revoke", handler.operations.RevokeTrust)
	case "/v1/topology":
		if request.Method != http.MethodGet {
			writeMethodNotAllowed(response)
			return
		}
		value, err := handler.operations.Topology(request.Context())
		if err != nil {
			writeInternalFailure(response)
			return
		}
		writeJSON(response, http.StatusOK, value)
	case "/v1/safe-stop":
		if request.Method != http.MethodPost || !emptyBody(request.Body) {
			writeMethodNotAllowed(response)
			return
		}
		value, err := handler.operations.SafeStop(request.Context())
		if err != nil {
			writeInternalFailure(response)
			return
		}
		writeJSON(response, http.StatusOK, value)
	default:
		writeResult(response, http.StatusNotFound, OperationResult{SchemaVersion: ResultSchemaVersion, Status: "REJECTED", Code: "not_found"})
	}
}

func emptyBody(reader io.Reader) bool {
	if reader == nil {
		return true
	}
	var value [1]byte
	count, err := reader.Read(value[:])
	return count == 0 && errors.Is(err, io.EOF)
}

func (handler *handler) operation(response http.ResponseWriter, request *http.Request, kind string, operation func(context.Context, OperationRequest) (OperationResult, error)) {
	if request.Method != http.MethodPost || !strings.HasPrefix(request.Header.Get("Content-Type"), "application/json") {
		writeMethodNotAllowed(response)
		return
	}
	var value OperationRequest
	if err := decodeStrict(request.Body, &value); err != nil || value.SchemaVersion != OperationSchemaVersion || validateOperationRequest(kind, value) != nil {
		writeResult(response, http.StatusBadRequest, OperationResult{SchemaVersion: ResultSchemaVersion, Status: "REJECTED", Code: "invalid_request"})
		return
	}
	result, err := operation(request.Context(), value)
	if err != nil {
		writeInternalFailure(response)
		return
	}
	writeJSON(response, http.StatusOK, result)
}

func validateOperationRequest(kind string, value OperationRequest) error {
	if kind == "pair" && (value.PairingAction == "PIN_LISTEN" || value.PairingAction == "PIN_DIAL") {
		if !validAlias(value.PeerAlias+"-00000000") || value.AllowInboundControl == nil || value.Fingerprint != "" {
			return errors.New("invalid PIN request")
		}
		if value.PairingAction == "PIN_LISTEN" {
			if value.Address != "" || value.PairingID != "" || value.ConfirmationCode != "" {
				return errors.New("invalid invitation")
			}
			return nil
		}
		if !validConfirmationCode(value.ConfirmationCode) || !validPairingID(value.PairingID) || (truststore.ConnectionSettings{Address: value.Address, Edge: "LEFT"}).Validate() != nil {
			return errors.New("invalid PIN join")
		}
		return nil
	}
	if value.Address != "" && (kind != "pair" || (value.PairingAction != "LISTEN" && value.PairingAction != "DIAL")) {
		return errors.New("unexpected address")
	}
	switch kind {
	case "connect":
		if !validAlias(value.PeerAlias) || value.PairingAction != "" || value.PairingID != "" || value.ConfirmationCode != "" || value.AllowInboundControl != nil || value.Fingerprint != "" {
			return errors.New("connect request is invalid")
		}
	case "disconnect":
		if value.PeerAlias != "" || value.PairingAction != "" || value.PairingID != "" || value.ConfirmationCode != "" || value.AllowInboundControl != nil || value.Fingerprint != "" {
			return errors.New("disconnect request is invalid")
		}
	case "pair":
		switch value.PairingAction {
		case "LISTEN", "DIAL":
			if !validAlias(value.PeerAlias) || value.AllowInboundControl == nil || value.PairingID != "" || value.ConfirmationCode != "" || (value.Fingerprint != "" && !validFingerprint(value.Fingerprint)) || (truststore.ConnectionSettings{Address: value.Address, Edge: "LEFT"}).Validate() != nil {
				return errors.New("pair network request is invalid")
			}
		case "STATUS":
			if value.PeerAlias != "" || value.AllowInboundControl != nil || !validPairingID(value.PairingID) || value.ConfirmationCode != "" || value.Fingerprint != "" {
				return errors.New("pair status request is invalid")
			}
		case "BEGIN":
			if !validAlias(value.PeerAlias) || value.AllowInboundControl == nil || value.PairingID != "" || value.ConfirmationCode != "" || value.Fingerprint != "" {
				return errors.New("pair begin request is invalid")
			}
		case "CONFIRM":
			if value.PeerAlias != "" || value.AllowInboundControl != nil || !validPairingID(value.PairingID) || !validConfirmationCode(value.ConfirmationCode) || value.Fingerprint != "" {
				return errors.New("pair confirm request is invalid")
			}
		case "CANCEL":
			if value.PeerAlias != "" || value.AllowInboundControl != nil || !validPairingID(value.PairingID) || value.ConfirmationCode != "" || value.Fingerprint != "" {
				return errors.New("pair cancel request is invalid")
			}
		default:
			return errors.New("pairing action is invalid")
		}
	case "trust-revoke":
		if !validFingerprint(value.Fingerprint) || value.PeerAlias != "" || value.PairingAction != "" || value.PairingID != "" || value.ConfirmationCode != "" || value.AllowInboundControl != nil {
			return errors.New("trust revoke request is invalid")
		}
	default:
		return errors.New("operation kind is invalid")
	}
	return nil
}

func validFingerprint(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validAlias(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) || utf8.RuneCountInString(value) > 64 {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validPairingID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validConfirmationCode(value string) bool {
	if len(value) != 6 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func (handler *handler) authenticated(header string) bool {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) || len(handler.token) != 64 {
		return false
	}
	provided := strings.TrimPrefix(header, prefix)
	return len(provided) == len(handler.token) && subtle.ConstantTimeCompare([]byte(provided), []byte(handler.token)) == 1
}

func decodeStrict(reader io.Reader, target any) error {
	data, err := io.ReadAll(io.LimitReader(reader, maxRequestBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxRequestBytes {
		return errors.New("request exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request contains trailing JSON")
	}
	return nil
}

func writeMethodNotAllowed(response http.ResponseWriter) {
	writeResult(response, http.StatusMethodNotAllowed, OperationResult{SchemaVersion: ResultSchemaVersion, Status: "REJECTED", Code: "method_not_allowed"})
}

func writeInternalFailure(response http.ResponseWriter) {
	writeResult(response, http.StatusInternalServerError, OperationResult{SchemaVersion: ResultSchemaVersion, Status: "FAILED", Code: "internal_failure"})
}

func writeResult(response http.ResponseWriter, status int, value OperationResult) {
	writeJSON(response, status, value)
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	var buffer bytes.Buffer
	if err := json.NewEncoder(&buffer).Encode(value); err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}
	response.WriteHeader(status)
	_, _ = response.Write(buffer.Bytes())
}
