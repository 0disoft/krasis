package labcli

import "testing"

func TestParsePeerCredentialsArgs(t *testing.T) {
	options, err := parsePeerCredentialsArgs([]string{"--output", "secrets", "--server-name=peer.test"})
	if err != nil {
		t.Fatalf("parsePeerCredentialsArgs() error = %v", err)
	}
	if options.outputDirectory != "secrets" || options.serverName != "peer.test" {
		t.Fatalf("options = %+v", options)
	}
}

func TestParsePeerCredentialsArgsRequiresBothValues(t *testing.T) {
	if _, err := parsePeerCredentialsArgs([]string{"--output", "secrets"}); err == nil {
		t.Fatal("parsePeerCredentialsArgs() accepted missing server name")
	}
}
