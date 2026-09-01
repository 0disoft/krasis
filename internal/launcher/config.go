package launcher

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/0disoft/krasis/internal/lab"
	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/securefs"
)

const (
	ConfigSchemaVersion = "krasis.launcher.config/v1"
	RoleListen          = "listen"
	RoleConnect         = "connect"
	DefaultPort         = 24876
	maxConfigBytes      = 64 << 10
)

type Config struct {
	SchemaVersion       string `json:"schemaVersion"`
	Role                string `json:"role"`
	DeviceAlias         string `json:"deviceAlias"`
	PeerHost            string `json:"peerHost"`
	Port                int    `json:"port"`
	Edge                string `json:"edge"`
	CredentialDirectory string `json:"credentialDirectory"`
	ReportsDirectory    string `json:"reportsDirectory"`
	Duration            string `json:"duration"`
}

type CommandSpec struct {
	Name string
	Args []string
}

type SessionPlan struct {
	Executable  string
	WorkingDir  string
	Commands    []CommandSpec
	AgentReport string
	GuardReport string
	StopFile    string
}

type CredentialPlan struct {
	Executable          string
	WorkingDir          string
	Commands            []CommandSpec
	PrepareDirectory    string
	CredentialDirectory string
	RevealDirectory     string
}

func DefaultConfig(hostname string) Config {
	config := Config{
		SchemaVersion:    ConfigSchemaVersion,
		Port:             DefaultPort,
		ReportsDirectory: "reports",
		Duration:         "2m",
	}
	if strings.EqualFold(strings.TrimSpace(hostname), "hp15") {
		config.Role = RoleListen
		config.DeviceAlias = "S-A"
		config.Edge = "RIGHT"
		config.CredentialDirectory = filepath.Join("secrets", "listener")
		return config
	}
	config.Role = RoleConnect
	config.DeviceAlias = "S-B"
	config.Edge = "LEFT"
	config.CredentialDirectory = filepath.Join("secrets", "connector")
	return config
}

func LoadConfig(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Config{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxConfigBytes {
		return Config{}, errors.New("launcher config must be a regular bounded file")
	}
	decoder := json.NewDecoder(io.LimitReader(file, maxConfigBytes+1))
	decoder.DisallowUnknownFields()
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Config{}, errors.New("launcher config contains multiple JSON values")
		}
		return Config{}, err
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func SaveConfig(path string, config Config) error {
	if err := config.Validate(); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	secureDirectory, err := securefs.OpenDirectory(directory)
	if err != nil {
		return err
	}
	defer secureDirectory.Close()
	temporary, temporaryName, err := secureDirectory.CreateTemporary(".launcher-config-", 0o600)
	if err != nil {
		return err
	}
	defer secureDirectory.Remove(temporaryName)
	if _, err := temporary.Write(payload); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return secureDirectory.Replace(temporaryName, filepath.Base(path))
}

func (config Config) Validate() error {
	if config.SchemaVersion != ConfigSchemaVersion {
		return errors.New("unsupported launcher config schema")
	}
	if !lab.ValidProbeDeviceAlias(config.DeviceAlias) {
		return errors.New("device alias must be H-A, H-B, S-A, or S-B")
	}
	if net.ParseIP(strings.TrimSpace(config.PeerHost)) == nil {
		return errors.New("peer host must be an IP address")
	}
	if config.Port < 1 || config.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	if _, err := time.ParseDuration(config.Duration); err != nil {
		return fmt.Errorf("invalid duration: %w", err)
	}
	duration, _ := time.ParseDuration(config.Duration)
	if duration < time.Second {
		return errors.New("duration must be at least one second")
	}
	switch config.Role {
	case RoleListen:
		if !strings.HasSuffix(config.DeviceAlias, "-A") || config.Edge != "RIGHT" {
			return errors.New("listener must use an A alias and RIGHT edge")
		}
	case RoleConnect:
		if !strings.HasSuffix(config.DeviceAlias, "-B") || config.Edge != "LEFT" {
			return errors.New("connector must use a B alias and LEFT edge")
		}
	default:
		return errors.New("role must be listen or connect")
	}
	for name, path := range map[string]string{
		"credential directory": config.CredentialDirectory,
		"reports directory":    config.ReportsDirectory,
	} {
		if err := validatePortableRelativePath(path); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func (config Config) BuildSessionPlan(base string, now time.Time) (SessionPlan, error) {
	if err := config.Validate(); err != nil {
		return SessionPlan{}, err
	}
	executable, err := requiredRegularFile(base, "krasis-lab.exe")
	if err != nil {
		return SessionPlan{}, err
	}
	credentials, err := containedPath(base, config.CredentialDirectory)
	if err != nil {
		return SessionPlan{}, err
	}
	ca, err := requiredRegularFile(credentials, "ca.pem")
	if err != nil {
		return SessionPlan{}, err
	}
	certificate, err := requiredRegularFile(credentials, "identity.pem")
	if err != nil {
		return SessionPlan{}, err
	}
	key, err := requiredRegularFile(credentials, "identity-key.pem")
	if err != nil {
		return SessionPlan{}, err
	}
	trust, err := containedPath(credentials, "trust")
	if err != nil {
		return SessionPlan{}, err
	}
	if info, err := os.Stat(trust); err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("not a directory")
		}
		return SessionPlan{}, fmt.Errorf("trust directory: %w", err)
	}
	reports, err := containedPath(base, config.ReportsDirectory)
	if err != nil {
		return SessionPlan{}, err
	}
	stamp := now.UTC().Format("20060102T150405.000000000Z")
	prefix := config.DeviceAlias + "-launcher-" + stamp
	agentReport := filepath.Join(reports, prefix+"-agent.json")
	guardReport := filepath.Join(reports, prefix+"-guard.json")
	stopFile := filepath.Join(reports, "."+prefix+".stop")
	address := net.JoinHostPort(config.PeerHost, strconv.Itoa(config.Port))
	args := []string{
		"agent-session", config.Role,
		"--device", config.DeviceAlias,
		"--address", address,
		"--ca", ca,
		"--cert", certificate,
		"--key", key,
		"--trust-store", trust,
		"--edge", config.Edge,
		"--duration", config.Duration,
		"--heartbeat-interval", "500ms",
		"--peer-timeout", "3s",
		"--output", agentReport,
		"--guard-output", guardReport,
		"--stop-file", stopFile,
		"--allow-live-input",
	}
	if config.Role == RoleConnect {
		args = append(args, "--server-name", config.PeerHost)
	}
	return SessionPlan{
		Executable: executable, WorkingDir: base,
		Commands:    []CommandSpec{{Name: "agent-session", Args: args}},
		AgentReport: agentReport, GuardReport: guardReport, StopFile: stopFile,
	}, nil
}

func (config Config) BuildCredentialPlan(base string, now time.Time) (CredentialPlan, error) {
	if err := config.Validate(); err != nil {
		return CredentialPlan{}, err
	}
	executable, err := requiredRegularFile(base, "krasis-lab.exe")
	if err != nil {
		return CredentialPlan{}, err
	}
	peerAlias, err := peerAlias(config.DeviceAlias)
	if err != nil {
		return CredentialPlan{}, err
	}
	if config.Role == RoleListen {
		pairRelative := filepath.Join("secrets", "pair-"+now.UTC().Format("20060102T150405.000000000Z"))
		pair, err := containedPath(base, pairRelative)
		if err != nil {
			return CredentialPlan{}, err
		}
		if _, err := os.Lstat(pair); err == nil {
			return CredentialPlan{}, errors.New("credential pair directory already exists")
		} else if !errors.Is(err, os.ErrNotExist) {
			return CredentialPlan{}, err
		}
		listener := filepath.Join(pair, "listener")
		return CredentialPlan{
			Executable: executable, WorkingDir: base,
			Commands: []CommandSpec{
				{Name: "peer-credentials", Args: []string{"peer-credentials", "--output", pair, "--server-name", config.PeerHost}},
				{Name: "protect-listener", Args: []string{"peer-credentials-protect", "--directory", listener}},
				{Name: "trust-listener", Args: []string{"trust", "import", "--store", filepath.Join(listener, "trust"), "--peer-cert", filepath.Join(listener, "peer.pem"), "--alias", peerAlias, "--allow-inbound-control"}},
			},
			PrepareDirectory:    filepath.Dir(pair),
			CredentialDirectory: filepath.Join(pairRelative, "listener"),
			RevealDirectory:     filepath.Join(pair, "connector"),
		}, nil
	}
	connectorRelative := config.CredentialDirectory
	connector, err := containedPath(base, connectorRelative)
	if err != nil {
		return CredentialPlan{}, err
	}
	if _, err := requiredRegularFile(connector, "peer.pem"); err != nil {
		return CredentialPlan{}, fmt.Errorf("copy the connector directory into secrets\\connector first: %w", err)
	}
	return CredentialPlan{
		Executable: executable, WorkingDir: base,
		Commands: []CommandSpec{
			{Name: "protect-connector", Args: []string{"peer-credentials-protect", "--directory", connector}},
			{Name: "trust-connector", Args: []string{"trust", "import", "--store", filepath.Join(connector, "trust"), "--peer-cert", filepath.Join(connector, "peer.pem"), "--alias", peerAlias, "--allow-inbound-control"}},
		},
		CredentialDirectory: connectorRelative,
	}, nil
}

func PrepareCredentialPlan(plan CredentialPlan) error {
	if plan.PrepareDirectory == "" {
		return nil
	}
	if err := os.MkdirAll(plan.PrepareDirectory, 0o700); err != nil {
		return fmt.Errorf("create credential parent directory: %w", err)
	}
	if _, err := peercredentials.ProtectSecurityDirectory(plan.PrepareDirectory); err != nil {
		return fmt.Errorf("protect credential parent directory: %w", err)
	}
	return nil
}

func validatePortableRelativePath(path string) error {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" || filepath.IsAbs(trimmed) || filepath.VolumeName(trimmed) != "" {
		return errors.New("path must be relative to the launcher directory")
	}
	cleaned := filepath.Clean(trimmed)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return errors.New("path must stay below the launcher directory")
	}
	return nil
}

func containedPath(base, relative string) (string, error) {
	if err := validatePortableRelativePath(relative); err != nil {
		return "", err
	}
	basePath, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(basePath, relative))
	if err != nil {
		return "", err
	}
	relativeToBase, err := filepath.Rel(basePath, target)
	if err != nil || relativeToBase == ".." || strings.HasPrefix(relativeToBase, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes launcher directory")
	}
	return target, nil
}

func requiredRegularFile(base, name string) (string, error) {
	path := filepath.Join(base, name)
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("required file %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("required file %s is not regular", name)
	}
	return path, nil
}

func peerAlias(alias string) (string, error) {
	switch alias {
	case "H-A":
		return "H-B", nil
	case "H-B":
		return "H-A", nil
	case "S-A":
		return "S-B", nil
	case "S-B":
		return "S-A", nil
	default:
		return "", errors.New("unsupported device alias")
	}
}
