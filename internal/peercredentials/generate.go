package peercredentials

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0disoft/krasis/internal/securefs"
)

const (
	SchemaVersion = "krasis.lab.peer-credentials/v3"
	Validity      = 24 * time.Hour
)

type Manifest struct {
	SchemaVersion            string    `json:"schemaVersion"`
	CreatedAt                time.Time `json:"createdAt"`
	ExpiresAt                time.Time `json:"expiresAt"`
	ServerName               string    `json:"serverName"`
	ListenerDir              string    `json:"listenerDirectory"`
	ConnectorDir             string    `json:"connectorDirectory"`
	ListenerPeerCertificate  string    `json:"listenerPeerCertificate"`
	ConnectorPeerCertificate string    `json:"connectorPeerCertificate"`
}

func Generate(outputDirectory, serverName string, now time.Time) (manifest Manifest, err error) {
	if outputDirectory == "" {
		return Manifest{}, errors.New("output directory is required")
	}
	if err := validateServerName(serverName); err != nil {
		return Manifest{}, err
	}
	if now.IsZero() {
		return Manifest{}, errors.New("creation time is required")
	}
	absolute, err := filepath.Abs(outputDirectory)
	if err != nil {
		return Manifest{}, fmt.Errorf("resolve output directory: %w", err)
	}
	if err := os.Mkdir(absolute, 0o700); err != nil {
		return Manifest{}, fmt.Errorf("create exclusive output directory: %w", err)
	}
	rootDirectory, err := securefs.OpenDirectory(absolute)
	if err != nil {
		return Manifest{}, fmt.Errorf("open credential output directory: %w", err)
	}
	if err := ProtectSecurityDirectoryHandle(rootDirectory.Handle()); err != nil {
		_ = rootDirectory.RemoveSelf()
		_ = rootDirectory.Close()
		return Manifest{}, err
	}
	listenerDirectory, err := rootDirectory.CreateDirectory("listener", 0o700)
	if err != nil {
		_ = rootDirectory.RemoveSelf()
		_ = rootDirectory.Close()
		return Manifest{}, err
	}
	if err := ProtectSecurityDirectoryHandle(listenerDirectory.Handle()); err != nil {
		_ = cleanupCredentialBundle(rootDirectory, listenerDirectory, nil)
		_ = listenerDirectory.Close()
		_ = rootDirectory.Close()
		return Manifest{}, err
	}
	connectorDirectory, err := rootDirectory.CreateDirectory("connector", 0o700)
	if err != nil {
		_ = cleanupCredentialBundle(rootDirectory, listenerDirectory, nil)
		_ = listenerDirectory.Close()
		_ = rootDirectory.Close()
		return Manifest{}, err
	}
	if err := ProtectSecurityDirectoryHandle(connectorDirectory.Handle()); err != nil {
		_ = cleanupCredentialBundle(rootDirectory, listenerDirectory, connectorDirectory)
		_ = connectorDirectory.Close()
		_ = listenerDirectory.Close()
		_ = rootDirectory.Close()
		return Manifest{}, err
	}
	completed := false
	defer func() {
		if !completed {
			if cleanupErr := cleanupCredentialBundle(rootDirectory, listenerDirectory, connectorDirectory); cleanupErr != nil {
				cleanupErr = fmt.Errorf("remove incomplete credential bundle: %w", cleanupErr)
				if err == nil {
					err = cleanupErr
				} else {
					err = errors.Join(err, cleanupErr)
				}
			}
		}
		_ = connectorDirectory.Close()
		_ = listenerDirectory.Close()
		_ = rootDirectory.Close()
	}()

	notBefore := now.UTC().Add(-time.Minute)
	expiresAt := now.UTC().Add(Validity)
	caKey, caCertificate, caDER, err := createCA(notBefore, expiresAt)
	if err != nil {
		return Manifest{}, err
	}
	listenerDER, listenerKey, err := issueIdentity(caCertificate, caKey, "listener", serverName, true, notBefore, expiresAt)
	if err != nil {
		return Manifest{}, err
	}
	connectorDER, connectorKey, err := issueIdentity(caCertificate, caKey, "connector", "", false, notBefore, expiresAt)
	if err != nil {
		return Manifest{}, err
	}
	for _, directory := range []*securefs.Directory{listenerDirectory, connectorDirectory} {
		if err := writePEMExclusive(directory, "ca.pem", "CERTIFICATE", caDER); err != nil {
			return Manifest{}, err
		}
	}
	if err := writeIdentity(listenerDirectory, listenerDER, listenerKey); err != nil {
		return Manifest{}, err
	}
	if err := writeIdentity(connectorDirectory, connectorDER, connectorKey); err != nil {
		return Manifest{}, err
	}
	if err := writePEMExclusive(listenerDirectory, "peer.pem", "CERTIFICATE", connectorDER); err != nil {
		return Manifest{}, err
	}
	if err := writePEMExclusive(connectorDirectory, "peer.pem", "CERTIFICATE", listenerDER); err != nil {
		return Manifest{}, err
	}
	manifest = Manifest{
		SchemaVersion: SchemaVersion,
		CreatedAt:     now.UTC(), ExpiresAt: expiresAt, ServerName: serverName,
		ListenerDir: "listener", ConnectorDir: "connector",
		ListenerPeerCertificate: "peer.pem", ConnectorPeerCertificate: "peer.pem",
	}
	if err := writeJSONExclusive(rootDirectory, "manifest.json", manifest); err != nil {
		return Manifest{}, err
	}
	completed = true
	return manifest, nil
}

func validateServerName(name string) error {
	if name == "" || strings.TrimSpace(name) != name {
		return errors.New("server name is required without surrounding whitespace")
	}
	if len(name) > 253 || strings.ContainsAny(name, `/\\`) || strings.ContainsRune(name, '\x00') {
		return errors.New("server name is invalid")
	}
	return nil
}

func createCA(notBefore, notAfter time.Time) (*ecdsa.PrivateKey, *x509.Certificate, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, nil, err
	}
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "Krasis peer probe CA"},
		NotBefore: notBefore, NotAfter: notAfter, IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, nil, err
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, nil, err
	}
	return key, certificate, der, nil
}

func issueIdentity(ca *x509.Certificate, caKey *ecdsa.PrivateKey, commonName, serverName string, server bool, notBefore, notAfter time.Time) ([]byte, *ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, err
	}
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "Krasis peer probe " + commonName},
		NotBefore: notBefore, NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature,
	}
	if server {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		if address := net.ParseIP(serverName); address != nil {
			template.IPAddresses = []net.IP{address}
		} else {
			template.DNSNames = []string{serverName}
		}
	} else {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}
	return der, key, nil
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, err
	}
	if serial.Sign() == 0 {
		return big.NewInt(1), nil
	}
	return serial, nil
}

func writeIdentity(directory *securefs.Directory, certificateDER []byte, key *ecdsa.PrivateKey) error {
	if err := writePEMExclusive(directory, "identity.pem", "CERTIFICATE", certificateDER); err != nil {
		return err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	return writePEMExclusive(directory, "identity-key.pem", "PRIVATE KEY", keyDER)
}

func writePEMExclusive(directory *securefs.Directory, name, blockType string, data []byte) error {
	file, err := directory.CreateExclusive(name, 0o600)
	if err != nil {
		return err
	}
	if err := ProtectSecurityFileHandle(file); err != nil {
		_ = file.Close()
		return err
	}
	if err := pem.Encode(file, &pem.Block{Type: blockType, Bytes: data}); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func writeJSONExclusive(directory *securefs.Directory, name string, value any) error {
	file, err := directory.CreateExclusive(name, 0o600)
	if err != nil {
		return err
	}
	if err := ProtectSecurityFileHandle(file); err != nil {
		_ = file.Close()
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func cleanupCredentialBundle(root, listener, connector *securefs.Directory) error {
	var cleanupErr error
	for _, directory := range []*securefs.Directory{listener, connector} {
		if directory == nil {
			continue
		}
		for _, name := range []string{"ca.pem", "identity.pem", "identity-key.pem", "peer.pem"} {
			if err := directory.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
				cleanupErr = errors.Join(cleanupErr, err)
			}
		}
		if err := directory.RemoveSelf(); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	if root != nil {
		if err := root.Remove("manifest.json"); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErr = errors.Join(cleanupErr, err)
		}
		if err := root.RemoveSelf(); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	return cleanupErr
}
