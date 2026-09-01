package localapi

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	StatusSchemaVersion      = "krasis.agent.status/v1"
	OperationSchemaVersion   = "krasis.agent.operation/v1"
	ResultSchemaVersion      = "krasis.agent.operation-result/v1"
	DiagnosticsSchemaVersion = "krasis.agent.diagnostics/v1"
	DefaultShutdownTimeout   = 5 * time.Second
	maxRequestBytes          = 64 << 10
)

type Status struct {
	SchemaVersion string   `json:"schemaVersion"`
	State         string   `json:"state"`
	PeerAlias     string   `json:"peerAlias,omitempty"`
	Actions       []string `json:"actions"`
}

type OperationRequest struct {
	SchemaVersion       string `json:"schemaVersion"`
	PeerAlias           string `json:"peerAlias,omitempty"`
	PairingAction       string `json:"pairingAction,omitempty"`
	PairingID           string `json:"pairingId,omitempty"`
	ConfirmationCode    string `json:"confirmationCode,omitempty"`
	AllowInboundControl *bool  `json:"allowInboundControl,omitempty"`
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

type Operations interface {
	Status(context.Context) (Status, error)
	Connect(context.Context, OperationRequest) (OperationResult, error)
	Disconnect(context.Context, OperationRequest) (OperationResult, error)
	Pair(context.Context, OperationRequest) (OperationResult, error)
	Diagnostics(context.Context) (Diagnostics, error)
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
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	credentials, err := PublishCredentials(controlDirectory, listener.Addr().String())
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	handler := &handler{token: credentials.Token(), operations: operations}
	return &Server{
		listener:    listener,
		credentials: credentials,
		http: &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: 2 * time.Second,
			ReadTimeout:       5 * time.Second,
			WriteTimeout:      5 * time.Second,
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
		closeErr := server.Close(context.Background())
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
		handler.operation(response, request, handler.operations.Connect)
	case "/v1/disconnect":
		handler.operation(response, request, handler.operations.Disconnect)
	case "/v1/pair":
		handler.operation(response, request, handler.operations.Pair)
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
	case "/v1/safe-stop":
		if request.Method != http.MethodPost || request.ContentLength > 0 {
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

func (handler *handler) operation(response http.ResponseWriter, request *http.Request, operation func(context.Context, OperationRequest) (OperationResult, error)) {
	if request.Method != http.MethodPost || !strings.HasPrefix(request.Header.Get("Content-Type"), "application/json") {
		writeMethodNotAllowed(response)
		return
	}
	var value OperationRequest
	if err := decodeStrict(request.Body, &value); err != nil || value.SchemaVersion != OperationSchemaVersion {
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
