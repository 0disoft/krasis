package launcher

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const testPeerHost = "192.0.2.10"

func TestDefaultConfigMatchesStudyPairRoles(t *testing.T) {
	listener := DefaultConfig("HP15")
	if listener.Role != RoleListen || listener.DeviceAlias != "S-A" || listener.Edge != "RIGHT" || listener.PeerHost != "" {
		t.Fatalf("listener defaults = %+v", listener)
	}
	connector := DefaultConfig("gram13")
	if connector.Role != RoleConnect || connector.DeviceAlias != "S-B" || connector.Edge != "LEFT" || connector.PeerHost != "" {
		t.Fatalf("connector defaults = %+v", connector)
	}
}

func TestConfigRejectsEscapingAndRoleMismatch(t *testing.T) {
	config := DefaultConfig("gram13")
	config.PeerHost = testPeerHost
	config.ReportsDirectory = filepath.Join("..", "reports")
	if err := config.Validate(); err == nil {
		t.Fatal("Validate accepted an escaping reports directory")
	}
	config = DefaultConfig("gram13")
	config.PeerHost = testPeerHost
	config.Edge = "RIGHT"
	if err := config.Validate(); err == nil {
		t.Fatal("Validate accepted a connector with a RIGHT edge")
	}
}

func TestLoadConfigRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "launcher-config.json")
	payload := `{"schemaVersion":"krasis.launcher.config/v1","role":"connect","deviceAlias":"S-B","peerHost":"192.0.2.10","port":24876,"edge":"LEFT","credentialDirectory":"secrets/connector","reportsDirectory":"reports","duration":"2m","secret":"leak"}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("LoadConfig accepted an unknown field")
	}
}

func TestBuildSessionPlanUsesExactRunPathsAndConnectorAddress(t *testing.T) {
	base := t.TempDir()
	config := DefaultConfig("gram13")
	config.PeerHost = testPeerHost
	createLauncherFixture(t, base, config.CredentialDirectory)
	now := time.Date(2026, 7, 17, 2, 3, 4, 5, time.UTC)
	plan, err := config.BuildSessionPlan(base, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Commands) != 1 || plan.StopFile == "" {
		t.Fatalf("session plan = %+v", plan)
	}
	args := plan.Commands[0].Args
	for _, expected := range []string{
		testPeerHost + ":24876", "--server-name", testPeerHost,
		"--stop-file", plan.StopFile, "--output", plan.AgentReport, "--guard-output", plan.GuardReport,
	} {
		if !contains(args, expected) {
			t.Fatalf("args do not contain %q: %v", expected, args)
		}
	}
	if filepath.Dir(plan.AgentReport) != filepath.Dir(plan.GuardReport) || filepath.Dir(plan.AgentReport) != filepath.Dir(plan.StopFile) {
		t.Fatalf("run outputs are not colocated: %+v", plan)
	}
	if plan.AgentReport == plan.GuardReport || plan.AgentReport == plan.StopFile || plan.GuardReport == plan.StopFile {
		t.Fatalf("run outputs are not distinct: %+v", plan)
	}
}

func TestBuildSessionPlanBindsListenerToSelectedInterface(t *testing.T) {
	base := t.TempDir()
	config := DefaultConfig("hp15")
	config.PeerHost = testPeerHost
	createLauncherFixture(t, base, config.CredentialDirectory)

	plan, err := config.BuildSessionPlan(base, time.Date(2026, 7, 17, 2, 3, 4, 5, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	args := plan.Commands[0].Args
	if !contains(args, testPeerHost+":24876") {
		t.Fatalf("listener args do not contain selected interface: %v", args)
	}
	if contains(args, "0.0.0.0:24876") || contains(args, "[::]:24876") {
		t.Fatalf("listener args contain a wildcard address: %v", args)
	}
}

func TestBuildCredentialPlanUsesConfiguredConnectorDirectory(t *testing.T) {
	base := t.TempDir()
	config := DefaultConfig("gram13")
	config.PeerHost = testPeerHost
	config.CredentialDirectory = filepath.Join("private", "connector")
	createLauncherFixture(t, base, config.CredentialDirectory)
	plan, err := config.BuildCredentialPlan(base, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if plan.CredentialDirectory != config.CredentialDirectory {
		t.Fatalf("credential directory = %q, want %q", plan.CredentialDirectory, config.CredentialDirectory)
	}
	if len(plan.Commands) != 2 || !contains(plan.Commands[0].Args, filepath.Join(base, config.CredentialDirectory)) {
		t.Fatalf("credential commands = %+v", plan.Commands)
	}
}

func TestPrepareCredentialPlanCreatesProtectedParentWithoutPrecreatingPair(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "krasis-lab.exe"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := DefaultConfig("hp15")
	config.PeerHost = testPeerHost
	plan, err := config.BuildCredentialPlan(base, time.Date(2026, 7, 17, 10, 34, 56, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(plan.PrepareDirectory); !os.IsNotExist(err) {
		t.Fatalf("credential parent unexpectedly exists before prepare: %v", err)
	}
	if err := PrepareCredentialPlan(plan); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(plan.PrepareDirectory); err != nil || !info.IsDir() {
		t.Fatalf("credential parent was not created: info=%v err=%v", info, err)
	}
	pairDirectory := filepath.Dir(plan.RevealDirectory)
	if _, err := os.Lstat(pairDirectory); !os.IsNotExist(err) {
		t.Fatalf("exclusive pair directory was precreated: %v", err)
	}
}

func TestPrepareCredentialPlanRejectsFileAtParentPath(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "secrets")
	if err := os.WriteFile(parent, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := PrepareCredentialPlan(CredentialPlan{PrepareDirectory: parent}); err == nil {
		t.Fatal("PrepareCredentialPlan accepted a file as the credential parent")
	}
}

func TestSaveAndLoadConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings", "launcher-config.json")
	want := DefaultConfig("hp15")
	want.PeerHost = testPeerHost
	if err := SaveConfig(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

func createLauncherFixture(t *testing.T, base, credentialsRelative string) {
	t.Helper()
	for _, path := range []string{
		filepath.Join(base, "krasis-lab.exe"),
		filepath.Join(base, credentialsRelative, "ca.pem"),
		filepath.Join(base, credentialsRelative, "identity.pem"),
		filepath.Join(base, credentialsRelative, "identity-key.pem"),
		filepath.Join(base, credentialsRelative, "peer.pem"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(base, credentialsRelative, "trust"), 0o700); err != nil {
		t.Fatal(err)
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}
