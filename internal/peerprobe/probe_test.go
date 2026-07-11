package peerprobe

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunCompletesTwoAuthenticatedFreshSessions(t *testing.T) {
	fixture := writeTLSFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ready := make(chan string, 1)
	type outcome struct {
		result Result
		err    error
	}
	listenerDone := make(chan outcome, 1)
	go func() {
		result, err := Run(ctx, Config{
			Role: RoleListen, DeviceAlias: "H-A", Address: "127.0.0.1:0",
			CAPath: fixture.ca, CertificatePath: fixture.serverCert, PrivateKeyPath: fixture.serverKey,
			Timeout: 5 * time.Second, Ready: func(address string) { ready <- address },
		})
		listenerDone <- outcome{result: result, err: err}
	}()
	var address string
	select {
	case address = <-ready:
	case <-ctx.Done():
		t.Fatalf("listener readiness: %v", ctx.Err())
	}
	connector, err := Run(ctx, Config{
		Role: RoleConnect, DeviceAlias: "H-B", Address: address, ExpectedServerName: "peer.test",
		CAPath: fixture.ca, CertificatePath: fixture.clientCert, PrivateKeyPath: fixture.clientKey,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("connector Run() error = %v, result=%+v", err, connector)
	}
	listener := <-listenerDone
	if listener.err != nil {
		t.Fatalf("listener Run() error = %v, result=%+v", listener.err, listener.result)
	}
	for role, result := range map[string]Result{"listener": listener.result, "connector": connector} {
		if !result.Completed || !result.Authenticated || !result.FreshIdentities {
			t.Fatalf("%s incomplete result: %+v", role, result)
		}
		if result.SessionsCompleted != SessionCount || result.LeaseRoundTrips != SessionCount || result.InputFramesChecked != SessionCount {
			t.Fatalf("%s counts = %+v", role, result)
		}
		if result.TLSVersion != "TLS1.3" || result.ALPN != "krasis/1" {
			t.Fatalf("%s TLS result = %+v", role, result)
		}
	}
}

func TestRunRejectsConnectWithoutExpectedServerName(t *testing.T) {
	result, err := Run(context.Background(), Config{
		Role: RoleConnect, Address: "127.0.0.1:1", CAPath: "ca", CertificatePath: "cert", PrivateKeyPath: "key", Timeout: time.Second,
	})
	if err == nil || result.Completed {
		t.Fatalf("Run() error = %v, result=%+v", err, result)
	}
}

type tlsFixture struct {
	ca, serverCert, serverKey, clientCert, clientKey string
}

func writeTLSFixture(t *testing.T) tlsFixture {
	t.Helper()
	directory := t.TempDir()
	caKey := mustKey(t)
	now := time.Now().Add(-time.Minute)
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Krasis peer probe test CA"},
		NotBefore: now, NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA: %v", err)
	}
	caPath := filepath.Join(directory, "ca.pem")
	writePEM(t, caPath, "CERTIFICATE", caDER)
	serverCert, serverKey := issueIdentity(t, directory, "server", caCert, caKey, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, []string{"peer.test"}, 2)
	clientCert, clientKey := issueIdentity(t, directory, "client", caCert, caKey, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, nil, 3)
	return tlsFixture{ca: caPath, serverCert: serverCert, serverKey: serverKey, clientCert: clientCert, clientKey: clientKey}
}

func issueIdentity(t *testing.T, directory, name string, ca *x509.Certificate, caKey *ecdsa.PrivateKey, usages []x509.ExtKeyUsage, names []string, serial int64) (string, string) {
	t.Helper()
	key := mustKey(t)
	now := time.Now().Add(-time.Minute)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: name}, DNSNames: names,
		NotBefore: now, NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: usages,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("issue %s certificate: %v", name, err)
	}
	certPath := filepath.Join(directory, name+".pem")
	keyPath := filepath.Join(directory, name+"-key.pem")
	writePEM(t, certPath, "CERTIFICATE", der)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal %s key: %v", name, err)
	}
	writePEM(t, keyPath, "PRIVATE KEY", keyDER)
	return certPath, keyPath
}

func mustKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

func writePEM(t *testing.T, path, kind string, data []byte) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	if err := pem.Encode(file, &pem.Block{Type: kind, Bytes: data}); err != nil {
		_ = file.Close()
		t.Fatalf("encode %s: %v", path, err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}
}
