package localapi

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/securefs"
)

const (
	EndpointSchemaVersion = "krasis.local-api-endpoint/v1"
	endpointFile          = "endpoint.json"
	tokenFile             = "auth-token"
	maxEndpointBytes      = 4 << 10
)

type Endpoint struct {
	SchemaVersion string `json:"schemaVersion"`
	Address       string `json:"address"`
}

type ClientCredentials struct {
	Endpoint Endpoint
	Token    string
}

type CredentialSession struct {
	directory *securefs.Directory
	token     string
	closed    bool
}

func PublishCredentials(directory, address string) (*CredentialSession, error) {
	if !validLoopbackAddress(address) {
		return nil, errors.New("local API address must be an IPv4 loopback host and port")
	}
	protected, err := prepareControlDirectory(directory)
	if err != nil {
		return nil, err
	}
	var random [32]byte
	if _, err := io.ReadFull(rand.Reader, random[:]); err != nil {
		_ = protected.Close()
		return nil, err
	}
	token := hex.EncodeToString(random[:])
	if err := replaceProtectedFile(protected, tokenFile, []byte(token+"\n")); err != nil {
		_ = protected.Close()
		return nil, err
	}
	endpoint, err := json.MarshalIndent(Endpoint{SchemaVersion: EndpointSchemaVersion, Address: address}, "", "  ")
	if err != nil {
		_ = protected.Remove(tokenFile)
		_ = protected.Close()
		return nil, err
	}
	if err := replaceProtectedFile(protected, endpointFile, append(endpoint, '\n')); err != nil {
		_ = protected.Remove(tokenFile)
		_ = protected.Close()
		return nil, err
	}
	return &CredentialSession{directory: protected, token: token}, nil
}

func (session *CredentialSession) Token() string {
	if session == nil || session.closed {
		return ""
	}
	return session.token
}

func (session *CredentialSession) Close() error {
	if session == nil || session.closed {
		return nil
	}
	session.closed = true
	var cleanup error
	for _, name := range []string{endpointFile, tokenFile} {
		if err := session.directory.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanup = errors.Join(cleanup, err)
		}
	}
	if err := session.directory.Close(); err != nil {
		cleanup = errors.Join(cleanup, err)
	}
	session.token = ""
	return cleanup
}

func LoadClientCredentials(directory string) (ClientCredentials, error) {
	if _, err := peercredentials.ValidateSecurityDirectory(directory); err != nil {
		return ClientCredentials{}, fmt.Errorf("validate local API directory: %w", err)
	}
	protected, err := securefs.OpenDirectory(directory)
	if err != nil {
		return ClientCredentials{}, err
	}
	defer protected.Close()
	endpointData, err := readProtectedFile(protected, endpointFile, maxEndpointBytes)
	if err != nil {
		return ClientCredentials{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(endpointData))
	decoder.DisallowUnknownFields()
	var endpoint Endpoint
	if err := decoder.Decode(&endpoint); err != nil {
		return ClientCredentials{}, fmt.Errorf("decode local API endpoint: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ClientCredentials{}, errors.New("local API endpoint contains trailing JSON")
	}
	if endpoint.SchemaVersion != EndpointSchemaVersion || !validLoopbackAddress(endpoint.Address) {
		return ClientCredentials{}, errors.New("local API endpoint is invalid")
	}
	tokenData, err := readProtectedFile(protected, tokenFile, 256)
	if err != nil {
		return ClientCredentials{}, err
	}
	token := strings.TrimSuffix(string(tokenData), "\n")
	if len(token) != 64 || strings.ToLower(token) != token {
		return ClientCredentials{}, errors.New("local API token is invalid")
	}
	if _, err := hex.DecodeString(token); err != nil {
		return ClientCredentials{}, errors.New("local API token is invalid")
	}
	return ClientCredentials{Endpoint: endpoint, Token: token}, nil
}

func prepareControlDirectory(path string) (*securefs.Directory, error) {
	if path == "" {
		return nil, errors.New("local API directory is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(absolute)
	if errors.Is(err, os.ErrNotExist) {
		parentPath := filepath.Dir(absolute)
		if _, err := peercredentials.ValidateSecurityDirectory(parentPath); err != nil {
			return nil, fmt.Errorf("validate product data root: %w", err)
		}
		parent, err := securefs.OpenDirectory(parentPath)
		if err != nil {
			return nil, err
		}
		defer parent.Close()
		directory, err := parent.CreateDirectory(filepath.Base(absolute), 0o700)
		if err != nil {
			return nil, err
		}
		if err := peercredentials.ProtectSecurityDirectoryHandle(directory.Handle()); err != nil {
			_ = directory.RemoveSelf()
			_ = directory.Close()
			return nil, err
		}
		return directory, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("local API path must be a directory")
	}
	if _, err := peercredentials.ValidateSecurityDirectory(absolute); err != nil {
		return nil, err
	}
	return securefs.OpenDirectory(absolute)
}

func replaceProtectedFile(directory *securefs.Directory, destination string, data []byte) error {
	temporary, name, err := directory.CreateTemporary(".control-", 0o600)
	if err != nil {
		return err
	}
	defer directory.Remove(name)
	if err := peercredentials.ProtectSecurityFileHandle(temporary); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return directory.Replace(name, destination)
}

func readProtectedFile(directory *securefs.Directory, name string, limit int64) ([]byte, error) {
	file, err := directory.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if err := peercredentials.ValidateSecurityFileHandle(file); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("local API credential file exceeds size limit")
	}
	return data, nil
}

func validLoopbackAddress(address string) bool {
	host, portText, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" {
		return false
	}
	port, err := strconv.Atoi(portText)
	return err == nil && port >= 1 && port <= 65535
}
