package labcli

import (
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/peerprobe"
)

func TestParsePeerProbeConnectArgs(t *testing.T) {
	options, err := parsePeerProbeArgs([]string{
		"connect", "--device", "H-B", "--address", "192.0.2.10:24876",
		"--server-name", "peer.test", "--ca", "ca.pem", "--cert", "client.pem",
		"--key", "client-key.pem", "--output", "report.json", "--timeout", "30s",
	})
	if err != nil {
		t.Fatalf("parsePeerProbeArgs() error = %v", err)
	}
	if options.role != peerprobe.RoleConnect || options.device != "H-B" || options.timeout != 30*time.Second {
		t.Fatalf("options = %+v", options)
	}
}

func TestParsePeerProbeListenRejectsServerName(t *testing.T) {
	_, err := parsePeerProbeArgs([]string{
		"listen", "--device", "H-A", "--address", ":24876", "--server-name", "peer.test",
		"--ca", "ca.pem", "--cert", "server.pem", "--key", "server-key.pem", "--output", "report.json",
	})
	if err == nil {
		t.Fatal("parsePeerProbeArgs() accepted --server-name in listen mode")
	}
}

func TestParsePeerProbeDefaultsTimeout(t *testing.T) {
	options, err := parsePeerProbeArgs([]string{
		"listen", "--device", "S-A", "--address", ":24876",
		"--ca", "ca.pem", "--cert", "server.pem", "--key", "server-key.pem", "--output", "report.json",
	})
	if err != nil {
		t.Fatalf("parsePeerProbeArgs() error = %v", err)
	}
	if options.timeout != 2*time.Minute {
		t.Fatalf("default timeout = %s", options.timeout)
	}
}
