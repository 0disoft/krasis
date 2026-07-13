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
	completed := false
	defer func() {
		if !completed {
			_ = os.RemoveAll(absolute)
		}
	}()

	listenerDirectory := filepath.Join(absolute, "listener")
	connectorDirectory := filepath.Join(absolute, "connector")
	if err := os.Mkdir(listenerDirectory, 0o700); err != nil {
		return Manifest{}, err
	}
	if err := os.Mkdir(connectorDirectory, 0o700); err != nil {
		return Manifest{}, err
	}

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
	for _, directory := range []string{listenerDirectory, connectorDirectory} {
		if err := writePEMExclusive(filepath.Join(directory, "ca.pem"), "CERTIFICATE", caDER); err != nil {
			return Manifest{}, err
		}
	}
	if err := writeIdentity(listenerDirectory, listenerDER, listenerKey); err != nil {
		return Manifest{}, err
	}
	if err := writeIdentity(connectorDirectory, connectorDER, connectorKey); err != nil {
		return Manifest{}, err
	}
	if err := writePEMExclusive(filepath.Join(listenerDirectory, "peer.pem"), "CERTIFICATE", connectorDER); err != nil {
		return Manifest{}, err
	}
	if err := writePEMExclusive(filepath.Join(connectorDirectory, "peer.pem"), "CERTIFICATE", listenerDER); err != nil {
		return Manifest{}, err
	}
	if err := ProtectPrivateKeyDirectory(listenerDirectory); err != nil {
		return Manifest{}, err
	}
	if err := ProtectPrivateKeyDirectory(connectorDirectory); err != nil {
		return Manifest{}, err
	}

	manifest = Manifest{
		SchemaVersion: SchemaVersion,
		CreatedAt:     now.UTC(), ExpiresAt: expiresAt, ServerName: serverName,
		ListenerDir: "listener", ConnectorDir: "connector",
		ListenerPeerCertificate: "peer.pem", ConnectorPeerCertificate: "peer.pem",
	}
	if err := writeJSONExclusive(filepath.Join(absolute, "manifest.json"), manifest); err != nil {
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

func writeIdentity(directory string, certificateDER []byte, key *ecdsa.PrivateKey) error {
	if err := writePEMExclusive(filepath.Join(directory, "identity.pem"), "CERTIFICATE", certificateDER); err != nil {
		return err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	return writePEMExclusive(filepath.Join(directory, "identity-key.pem"), "PRIVATE KEY", keyDER)
}

func writePEMExclusive(path, blockType string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if err := pem.Encode(file, &pem.Block{Type: blockType, Bytes: data}); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func writeJSONExclusive(path string, value any) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
