package localapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const maxResponseBytes = 1 << 20

var (
	ErrUnavailable    = errors.New("resident agent is unavailable")
	ErrAuthentication = errors.New("local API authentication failed")
	ErrTransport      = errors.New("local API transport failed")
	ErrProtocol       = errors.New("local API protocol failed")
	ErrNotFound       = fmt.Errorf("%w: endpoint not found", ErrProtocol)
)

type Client struct {
	credentials ClientCredentials
	http        *http.Client
}

func OpenClient(controlDirectory string) (*Client, error) {
	credentials, err := LoadClientCredentials(controlDirectory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: endpoint credentials are absent", ErrUnavailable)
		}
		return nil, fmt.Errorf("%w: endpoint credentials are invalid", ErrAuthentication)
	}
	return &Client{credentials: credentials, http: &http.Client{Timeout: clientRequestTimeout}}, nil
}

func (client *Client) Status(ctx context.Context) (Status, error) {
	var value Status
	err := client.get(ctx, "/v1/status", &value)
	if err == nil && value.SchemaVersion != StatusSchemaVersion {
		err = ErrProtocol
	}
	return value, err
}

func (client *Client) Diagnostics(ctx context.Context) (Diagnostics, error) {
	var value Diagnostics
	err := client.get(ctx, "/v1/diagnostics", &value)
	if err == nil && value.SchemaVersion != DiagnosticsSchemaVersion {
		err = ErrProtocol
	}
	return value, err
}

func (client *Client) Trust(ctx context.Context) (Trust, error) {
	var value Trust
	err := client.get(ctx, "/v1/trust", &value)
	if err == nil && (value.SchemaVersion != TrustSchemaVersion || value.Peers == nil) {
		err = ErrProtocol
	}
	return value, err
}

func (client *Client) Topology(ctx context.Context) (Topology, error) {
	var value Topology
	err := client.get(ctx, "/v1/topology", &value)
	if err == nil && (value.SchemaVersion != TopologySchemaVersion || value.Peers == nil) {
		err = ErrProtocol
	}
	return value, err
}

func (client *Client) Connect(ctx context.Context, alias string) (OperationResult, error) {
	return client.operation(ctx, "/v1/connect", OperationRequest{SchemaVersion: OperationSchemaVersion, PeerAlias: alias})
}

func (client *Client) Disconnect(ctx context.Context) (OperationResult, error) {
	return client.operation(ctx, "/v1/disconnect", OperationRequest{SchemaVersion: OperationSchemaVersion})
}

func (client *Client) Pair(ctx context.Context, request OperationRequest) (OperationResult, error) {
	request.SchemaVersion = OperationSchemaVersion
	return client.operation(ctx, "/v1/pair", request)
}

func (client *Client) RevokeTrust(ctx context.Context, fingerprint string) (OperationResult, error) {
	return client.operation(ctx, "/v1/trust/revoke", OperationRequest{SchemaVersion: OperationSchemaVersion, Fingerprint: fingerprint})
}

func (client *Client) SafeStop(ctx context.Context) (OperationResult, error) {
	var value OperationResult
	err := client.request(ctx, http.MethodPost, "/v1/safe-stop", nil, &value)
	if err == nil && value.SchemaVersion != ResultSchemaVersion {
		err = ErrProtocol
	}
	return value, err
}

func (client *Client) operation(ctx context.Context, path string, request OperationRequest) (OperationResult, error) {
	var value OperationResult
	err := client.request(ctx, http.MethodPost, path, request, &value)
	if err == nil && value.SchemaVersion != ResultSchemaVersion {
		err = ErrProtocol
	}
	return value, err
}

func (client *Client) get(ctx context.Context, path string, target any) error {
	return client.request(ctx, http.MethodGet, path, nil, target)
}

func (client *Client) request(ctx context.Context, method, path string, body, target any) error {
	return client.requestHeaders(ctx, method, path, body, target, nil)
}

func (client *Client) requestHeaders(ctx context.Context, method, path string, body, target any, headers map[string]string) error {
	return client.requestHeadersLimit(ctx, method, path, body, target, headers, maxResponseBytes)
}

func (client *Client) requestHeadersLimit(ctx context.Context, method, path string, body, target any, headers map[string]string, responseLimit int64) error {
	if client == nil || client.http == nil || client.credentials.Token == "" {
		return ErrUnavailable
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("%w: encode request", ErrProtocol)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://"+client.credentials.Endpoint.Address+path, reader)
	if err != nil {
		return fmt.Errorf("%w: create request", ErrProtocol)
	}
	request.Header.Set("Authorization", "Bearer "+client.credentials.Token)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.http.Do(request)
	if err != nil {
		return fmt.Errorf("%w: request failed", ErrUnavailable)
	}
	defer response.Body.Close()
	if !strings.HasPrefix(response.Header.Get("Content-Type"), "application/json") {
		return fmt.Errorf("%w: unexpected content type", ErrProtocol)
	}
	if response.StatusCode == http.StatusUnauthorized {
		return ErrAuthentication
	}
	if response.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%w: HTTP status %d", ErrProtocol, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, responseLimit+1))
	if err != nil {
		return fmt.Errorf("%w: read response", ErrTransport)
	}
	if int64(len(data)) > responseLimit {
		return fmt.Errorf("%w: response exceeds size limit", ErrProtocol)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: decode response", ErrProtocol)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing response", ErrProtocol)
	}
	return nil
}
