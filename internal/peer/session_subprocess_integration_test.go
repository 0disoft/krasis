package peer

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

const (
	peerProcessRoleEnv      = "KRASIS_TEST_PEER_ROLE"
	peerProcessAddress      = "KRASIS_TEST_PEER_ADDRESS"
	peerProcessCA           = "KRASIS_TEST_PEER_CA"
	peerProcessCert         = "KRASIS_TEST_PEER_CERT"
	peerProcessKey          = "KRASIS_TEST_PEER_KEY"
	peerProcessScenario     = "KRASIS_TEST_PEER_SCENARIO"
	peerProcessReadyTag     = "KRASIS_PEER_READY="
	peerProcessActiveTag    = "KRASIS_PEER_ACTIVE="
	peerProcessRecoveredTag = "KRASIS_PEER_RECOVERED="
	peerTestServerName      = "peer.test"
)

func TestPeerSourceRecoversWhenDestinationProcessIsKilled(t *testing.T) {
	fixture := writeProcessTLSFixture(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	destination := startMonitoredPeerProcess(t, ctx, executable, []string{
		peerProcessRoleEnv + "=destination",
		peerProcessScenario + "=kill_destination",
		peerProcessCA + "=" + fixture.ca,
		peerProcessCert + "=" + fixture.serverCert,
		peerProcessKey + "=" + fixture.serverKey,
	})
	address := waitPeerProcessTag(t, ctx, destination.events, peerProcessReadyTag)
	source := startMonitoredPeerProcess(t, ctx, executable, []string{
		peerProcessRoleEnv + "=source",
		peerProcessScenario + "=kill_destination",
		peerProcessAddress + "=" + address,
		peerProcessCA + "=" + fixture.ca,
		peerProcessCert + "=" + fixture.clientCert,
		peerProcessKey + "=" + fixture.clientKey,
	})
	if got := waitPeerProcessTag(t, ctx, destination.events, peerProcessActiveTag); got != "destination" {
		t.Fatalf("active process = %q", got)
	}
	if err := destination.command.Process.Kill(); err != nil {
		t.Fatalf("kill destination process: %v", err)
	}
	destinationOutput, destinationErr := destination.wait()
	if destinationErr == nil {
		t.Fatalf("killed destination exited successfully:\n%s", destinationOutput)
	}
	sourceOutput, sourceErr := source.wait()
	if sourceErr != nil || !strings.Contains(sourceOutput, peerProcessRecoveredTag+"source") {
		t.Fatalf("source did not recover: error=%v\nsource output:\n%s\ndestination output:\n%s", sourceErr, sourceOutput, destinationOutput)
	}
}

func TestPeerDestinationRecoversWhenSourceProcessIsKilled(t *testing.T) {
	fixture := writeProcessTLSFixture(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	destination := startMonitoredPeerProcess(t, ctx, executable, []string{
		peerProcessRoleEnv + "=destination",
		peerProcessScenario + "=kill_source",
		peerProcessCA + "=" + fixture.ca,
		peerProcessCert + "=" + fixture.serverCert,
		peerProcessKey + "=" + fixture.serverKey,
	})
	address := waitPeerProcessTag(t, ctx, destination.events, peerProcessReadyTag)
	source := startMonitoredPeerProcess(t, ctx, executable, []string{
		peerProcessRoleEnv + "=source",
		peerProcessScenario + "=kill_source",
		peerProcessAddress + "=" + address,
		peerProcessCA + "=" + fixture.ca,
		peerProcessCert + "=" + fixture.clientCert,
		peerProcessKey + "=" + fixture.clientKey,
	})
	if got := waitPeerProcessTag(t, ctx, destination.events, peerProcessActiveTag); got != "destination" {
		t.Fatalf("active process = %q", got)
	}
	if err := source.command.Process.Kill(); err != nil {
		t.Fatalf("kill source process: %v", err)
	}
	sourceOutput, sourceErr := source.wait()
	if sourceErr == nil {
		t.Fatalf("killed source exited successfully:\n%s", sourceOutput)
	}
	destinationOutput, destinationErr := destination.wait()
	if destinationErr != nil || !strings.Contains(destinationOutput, peerProcessRecoveredTag+"destination") {
		t.Fatalf("destination did not recover: error=%v\ndestination output:\n%s\nsource output:\n%s", destinationErr, destinationOutput, sourceOutput)
	}
}

func TestPeerSessionsRoundTripAcrossTLSPeerProcesses(t *testing.T) {
	fixture := writeProcessTLSFixture(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	destination := exec.CommandContext(ctx, executable, "-test.run=^TestPeerTLSProcessHelper$", "-test.v")
	destination.Env = append(os.Environ(),
		peerProcessRoleEnv+"=destination",
		peerProcessCA+"="+fixture.ca,
		peerProcessCert+"="+fixture.serverCert,
		peerProcessKey+"="+fixture.serverKey,
	)
	destinationStdout, err := destination.StdoutPipe()
	if err != nil {
		t.Fatalf("destination StdoutPipe() error = %v", err)
	}
	var destinationStderr bytes.Buffer
	destination.Stderr = &destinationStderr
	ready := make(chan string, 1)
	outputDone := make(chan string, 1)
	go func() {
		var output strings.Builder
		scanner := bufio.NewScanner(destinationStdout)
		for scanner.Scan() {
			line := scanner.Text()
			output.WriteString(line)
			output.WriteByte('\n')
			if address, found := strings.CutPrefix(line, peerProcessReadyTag); found {
				select {
				case ready <- address:
				default:
				}
			}
		}
		outputDone <- output.String()
	}()
	if err := destination.Start(); err != nil {
		t.Fatalf("start destination process: %v", err)
	}

	var address string
	select {
	case address = <-ready:
	case <-ctx.Done():
		_ = destination.Process.Kill()
		_ = destination.Wait()
		t.Fatalf("destination readiness timeout: %v", ctx.Err())
	}

	source := exec.CommandContext(ctx, executable, "-test.run=^TestPeerTLSProcessHelper$", "-test.v")
	source.Env = append(os.Environ(),
		peerProcessRoleEnv+"=source",
		peerProcessAddress+"="+address,
		peerProcessCA+"="+fixture.ca,
		peerProcessCert+"="+fixture.clientCert,
		peerProcessKey+"="+fixture.clientKey,
	)
	sourceOutput, sourceErr := source.CombinedOutput()
	destinationErr := destination.Wait()
	destinationOutput := <-outputDone
	if sourceErr != nil || destinationErr != nil {
		t.Fatalf(
			"TLS peer subprocess round trip failed: source=%v destination=%v\nsource output:\n%s\ndestination output:\n%s\ndestination stderr:\n%s",
			sourceErr,
			destinationErr,
			sourceOutput,
			destinationOutput,
			destinationStderr.String(),
		)
	}
}

func TestPeerTLSProcessHelper(t *testing.T) {
	role := os.Getenv(peerProcessRoleEnv)
	if role == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	switch role {
	case "destination":
		runDestinationPeerProcess(t, ctx)
	case "source":
		runSourcePeerProcess(t, ctx)
	default:
		t.Fatalf("unknown peer process role %q", role)
	}
}

func runDestinationPeerProcess(t *testing.T, ctx context.Context) {
	serverConfig := loadProcessServerTLSConfig(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	fmt.Printf("%s%s\n", peerProcessReadyTag, listener.Addr().String())
	connection, err := listener.Accept()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	peerConnection, err := transport.NewPeerConnection(ctx, tls.Server(connection, serverConfig))
	if err != nil {
		t.Fatalf("NewPeerConnection(server) error = %v", err)
	}
	defer peerConnection.Close()

	input := &integrationInputPort{
		moves:    make(chan [2]int32, 1),
		presses:  make(chan control.PressedToken, 1),
		releases: make(chan control.PressedToken, 1),
	}
	runtime := newIntegrationRuntime(t, input)
	coordinator := testWriteCoordinator(t, peerConnection)
	returns := &integrationReturnBoundary{}
	responder := &LeaseResponder{
		Coordinator: coordinator,
		Runtime:     runtime,
		Authorizer:  staticLeaseAuthorizer{leaseID: "lease-1", accepted: true},
		Entry:       integrationCursorEntry{},
		Returns:     returns,
	}
	returnController, err := NewReturnController(coordinator, runtime, returns)
	if err != nil {
		t.Fatalf("NewReturnController() error = %v", err)
	}
	sessionDone := make(chan error, 1)
	go func() {
		sessionDone <- RunSessionWithLifecycle(ctx, peerConnection, runtime, responder, nil, time.Now)
	}()
	select {
	case move := <-input.moves:
		if move != [2]int32{7, -3} {
			t.Fatalf("destination move = %v", move)
		}
	case <-ctx.Done():
		t.Fatalf("destination input timeout: %v", ctx.Err())
	}
	scenario := os.Getenv(peerProcessScenario)
	if scenario == "kill_destination" {
		fmt.Printf("%sdestination\n", peerProcessActiveTag)
		<-ctx.Done()
		return
	}
	if scenario == "kill_source" {
		select {
		case token := <-input.presses:
			if token.Kind != control.PressedKeyboardKey || token.Code != 1 {
				t.Fatalf("destination press = %+v", token)
			}
		case <-ctx.Done():
			t.Fatalf("destination key press timeout: %v", ctx.Err())
		}
		fmt.Printf("%sdestination\n", peerProcessActiveTag)
		select {
		case <-sessionDone:
		case <-ctx.Done():
			t.Fatalf("destination recovery timeout: %v", ctx.Err())
		}
		if runtime.Snapshot().Control.State != control.StateLocal {
			t.Fatalf("destination did not recover local: %s", runtime.Snapshot())
		}
		select {
		case token := <-input.releases:
			if token.Kind != control.PressedKeyboardKey || token.Code != 1 {
				t.Fatalf("destination release = %+v", token)
			}
		default:
			t.Fatal("destination cleanup did not release held key")
		}
		fmt.Printf("%sdestination\n", peerProcessRecoveredTag)
		return
	}
	if err := returnController.Revoke(ctx, control.ReturnIntent{SessionID: "session-1", Epoch: "epoch-1"}); err != nil {
		t.Fatalf("return Revoke() error = %v", err)
	}
	if runtime.Snapshot().Control.State != control.StateLocal {
		t.Fatalf("destination runtime = %s", runtime.Snapshot())
	}
	select {
	case <-sessionDone:
	case <-ctx.Done():
		t.Fatalf("destination session shutdown timeout: %v", ctx.Err())
	}
}

func runSourcePeerProcess(t *testing.T, ctx context.Context) {
	clientConfig := loadProcessClientTLSConfig(t)
	connection, err := net.DialTimeout("tcp", requiredProcessEnv(t, peerProcessAddress), 3*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	peerConnection, err := transport.NewPeerConnection(ctx, tls.Client(connection, clientConfig))
	if err != nil {
		t.Fatalf("NewPeerConnection(client) error = %v", err)
	}
	defer peerConnection.Close()

	runtime := newIntegrationRuntime(t, &integrationInputPort{moves: make(chan [2]int32, 1)})
	coordinator := testWriteCoordinator(t, peerConnection)
	activation := newIntegrationSourceActivation()
	outbound, err := NewOutboundController(coordinator, runtime, activation)
	if err != nil {
		t.Fatalf("NewOutboundController() error = %v", err)
	}
	sessionDone := make(chan error, 1)
	go func() {
		sessionDone <- RunSessionWithLifecycle(ctx, peerConnection, runtime, nil, outbound, time.Now)
	}()
	if err := outbound.BeginBoundary(
		ctx,
		control.BoundaryIntent{Edge: control.ScreenEdgeRight, X: 1919},
		OutboundIdentity{SessionID: "session-1", Epoch: "epoch-1"},
	); err != nil {
		t.Fatalf("BeginBoundary() error = %v", err)
	}
	waitSignal(t, activation.activated, "TLS source activation")
	if err := SendInputAction(ctx, coordinator, runtime, control.InputAction{Kind: control.InputPointerMove, DeltaX: 7, DeltaY: -3}); err != nil {
		t.Fatalf("SendInputAction() error = %v", err)
	}
	scenario := os.Getenv(peerProcessScenario)
	if scenario == "kill_source" {
		if err := SendInputAction(ctx, coordinator, runtime, control.InputAction{Kind: control.InputKeyDown, Code: 1}); err != nil {
			t.Fatalf("SendInputAction(key down) error = %v", err)
		}
		fmt.Printf("%ssource\n", peerProcessActiveTag)
		<-ctx.Done()
		return
	}
	if scenario == "kill_destination" {
		fmt.Printf("%ssource\n", peerProcessActiveTag)
		select {
		case <-sessionDone:
		case <-ctx.Done():
			t.Fatalf("source recovery timeout: %v", ctx.Err())
		}
		waitSignal(t, activation.deactivated, "source suppression recovery")
		if runtime.Snapshot().Control.State != control.StateLocal || activation.Active() {
			t.Fatalf("source did not recover local: runtime=%s active=%t", runtime.Snapshot(), activation.Active())
		}
		fmt.Printf("%ssource\n", peerProcessRecoveredTag)
		return
	}
	waitSignal(t, activation.deactivated, "TLS source return revoke")
	if runtime.Snapshot().Control.State != control.StateLocal {
		t.Fatalf("source runtime = %s", runtime.Snapshot())
	}
	_ = peerConnection.Close()
	select {
	case <-sessionDone:
	case <-ctx.Done():
		t.Fatalf("source session shutdown timeout: %v", ctx.Err())
	}
}

type monitoredPeerProcess struct {
	command    *exec.Cmd
	events     <-chan string
	outputDone <-chan string
	stderr     *bytes.Buffer
}

func startMonitoredPeerProcess(
	t *testing.T,
	ctx context.Context,
	executable string,
	environment []string,
) *monitoredPeerProcess {
	t.Helper()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestPeerTLSProcessHelper$", "-test.v")
	command.Env = append(os.Environ(), environment...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("peer StdoutPipe() error = %v", err)
	}
	stderr := &bytes.Buffer{}
	command.Stderr = stderr
	events := make(chan string, 8)
	outputDone := make(chan string, 1)
	go func() {
		var output strings.Builder
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			output.WriteString(line)
			output.WriteByte('\n')
			if strings.HasPrefix(line, "KRASIS_PEER_") {
				events <- line
			}
		}
		close(events)
		outputDone <- output.String()
	}()
	if err := command.Start(); err != nil {
		t.Fatalf("start peer process: %v", err)
	}
	return &monitoredPeerProcess{command: command, events: events, outputDone: outputDone, stderr: stderr}
}

func waitPeerProcessTag(t *testing.T, ctx context.Context, events <-chan string, prefix string) string {
	t.Helper()
	for {
		select {
		case line, ok := <-events:
			if !ok {
				t.Fatalf("peer process exited before %s", prefix)
			}
			if value, found := strings.CutPrefix(line, prefix); found {
				return value
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %s: %v", prefix, ctx.Err())
		}
	}
}

func (process *monitoredPeerProcess) wait() (string, error) {
	err := process.command.Wait()
	output := <-process.outputDone
	if process.stderr.Len() != 0 {
		output += process.stderr.String()
	}
	return output, err
}

type processTLSFixture struct {
	ca         string
	serverCert string
	serverKey  string
	clientCert string
	clientKey  string
}

func writeProcessTLSFixture(t *testing.T) processTLSFixture {
	t.Helper()
	directory := t.TempDir()
	now := time.Now()
	caKey := generateProcessKey(t)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Krasis subprocess test CA"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create process test CA: %v", err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse process test CA: %v", err)
	}
	caPath := filepath.Join(directory, "ca.pem")
	writePEMFile(t, caPath, &pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	serverCert, serverKey := writeProcessIdentity(t, directory, "server", ca, caKey, big.NewInt(2), peerTestServerName, x509.ExtKeyUsageServerAuth)
	clientCert, clientKey := writeProcessIdentity(t, directory, "client", ca, caKey, big.NewInt(3), "client", x509.ExtKeyUsageClientAuth)
	return processTLSFixture{ca: caPath, serverCert: serverCert, serverKey: serverKey, clientCert: clientCert, clientKey: clientKey}
}

func writeProcessIdentity(
	t *testing.T,
	directory string,
	name string,
	ca *x509.Certificate,
	caKey *ecdsa.PrivateKey,
	serial *big.Int,
	commonName string,
	usage x509.ExtKeyUsage,
) (string, string) {
	t.Helper()
	key := generateProcessKey(t)
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		DNSNames:     []string{commonName},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("issue %s identity: %v", name, err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal %s key: %v", name, err)
	}
	certPath := filepath.Join(directory, name+".pem")
	keyPath := filepath.Join(directory, name+"-key.pem")
	writePEMFile(t, certPath,
		&pem.Block{Type: "CERTIFICATE", Bytes: der},
		&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw},
	)
	writePEMFile(t, keyPath, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPath, keyPath
}

func generateProcessKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate process key: %v", err)
	}
	return key
}

func writePEMFile(t *testing.T, path string, blocks ...*pem.Block) {
	t.Helper()
	var data bytes.Buffer
	for _, block := range blocks {
		if err := pem.Encode(&data, block); err != nil {
			t.Fatalf("encode PEM %s: %v", path, err)
		}
	}
	if err := os.WriteFile(path, data.Bytes(), 0o600); err != nil {
		t.Fatalf("write PEM %s: %v", path, err)
	}
}

func loadProcessServerTLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	identity := loadProcessIdentity(t)
	roots := loadProcessRoots(t)
	config, err := transport.NewServerTLSConfig(identity, roots)
	if err != nil {
		t.Fatalf("NewServerTLSConfig() error = %v", err)
	}
	return config
}

func loadProcessClientTLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	identity := loadProcessIdentity(t)
	roots := loadProcessRoots(t)
	config, err := transport.NewClientTLSConfig(identity, roots, peerTestServerName)
	if err != nil {
		t.Fatalf("NewClientTLSConfig() error = %v", err)
	}
	return config
}

func loadProcessIdentity(t *testing.T) tls.Certificate {
	t.Helper()
	identity, err := tls.LoadX509KeyPair(requiredProcessEnv(t, peerProcessCert), requiredProcessEnv(t, peerProcessKey))
	if err != nil {
		t.Fatalf("LoadX509KeyPair() error = %v", err)
	}
	return identity
}

func loadProcessRoots(t *testing.T) *x509.CertPool {
	t.Helper()
	data, err := os.ReadFile(requiredProcessEnv(t, peerProcessCA))
	if err != nil {
		t.Fatalf("read test CA: %v", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(data) {
		t.Fatal("test CA PEM was not accepted")
	}
	return roots
}

func requiredProcessEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("required process environment %s is empty", name)
	}
	return value
}
