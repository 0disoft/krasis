package productcli

const helpSchema = "krasis.cli.help/v1"

type helpCommand struct {
	Usage         string `json:"usage"`
	Summary       string `json:"summary"`
	RequiresAgent bool   `json:"requiresAgent"`
	ChangesState  bool   `json:"changesState"`
}

type helpResult struct {
	SchemaVersion string        `json:"schemaVersion"`
	Commands      []helpCommand `json:"commands"`
}

// Keep each callable leaf visible to scripts; a parent command can mix reads and writes.
func commandHelp() helpResult {
	return helpResult{SchemaVersion: helpSchema, Commands: []helpCommand{
		{"help", "List commands and their effects", false, false},
		{"version", "Show CLI build and optional running agent build", false, false},
		{"relaunch", "Open the last product UI without a running agent", false, true},
		{"app open ALIAS", "Open the remembered UI on a trusted PC", true, true},
		{"app open-status ALIAS ID", "Read a peer UI launch request result", true, false},
		{"doctor", "Check agent protocol and connection", true, false},
		{"status", "Read connection state", true, false},
		{"topology", "Read device layout", true, false},
		{"diagnostics", "Read live diagnostic counters", true, false},
		{"diagnostics status", "Read latest local diagnostic transfer state", true, false},
		{"diagnostics export", "Save sanitized local support file", false, true},
		{"diagnostics send ALIAS", "Queue sanitized diagnostics to a trusted peer", true, true},
		{"diagnostics collect ALIAS", "Export both PCs' diagnostics to this PC", true, true},
		{"diagnostics collect-status ALIAS ID", "Read a collection request's progress", true, false},
		{"trust list", "List trusted devices", true, false},
		{"connection show ALIAS", "Read saved peer address and exit edge", true, false},
		{"credential show LISTENER|CONNECTOR", "Read local credential status", true, false},
		{"update verify", "Validate adjacent update bundle offline", false, false},
		{"update status", "Read local update transfer state", true, false},
		{"update status ALIAS", "Read peer version and update permission", true, false},
		{"clipboard status ALIAS", "Read clipboard sharing state", true, false},
		{"pair status PAIRING_ID", "Read pairing job state", true, false},
		{"connect ALIAS", "Connect to a configured peer", true, true},
		{"disconnect", "Disconnect the active peer", true, true},
		{"safe-stop", "Stop agent and active session", true, true},
		{"connection set ALIAS IP:PORT LEFT|RIGHT", "Save peer address and exit edge", true, true},
		{"credential renew LISTENER|CONNECTOR EXPECTED_FINGERPRINT", "Renew local credential", true, true},
		{"clipboard on ALIAS", "Share future clipboard content", true, true},
		{"clipboard off ALIAS", "Stop clipboard sharing", true, true},
		{"update send ALIAS", "Queue current bundle to a trusted peer", true, true},
		{"update allow ALIAS", "Allow peer updates to this PC", true, true},
		{"update deny ALIAS", "Block peer updates to this PC", true, true},
		{"trust revoke FINGERPRINT", "Revoke trusted device", true, true},
		{"pair listen ALIAS IP:PORT --allow-inbound-control|--deny-inbound-control [--replace FINGERPRINT]", "Wait for explicit pairing", true, true},
		{"pair dial ALIAS IP:PORT --allow-inbound-control|--deny-inbound-control [--replace FINGERPRINT]", "Start explicit pairing", true, true},
		{"pair begin ALIAS --allow-inbound-control", "Start local pairing job", true, true},
		{"pair confirm PAIRING_ID CODE", "Confirm pairing code", true, true},
		{"pair cancel PAIRING_ID", "Cancel pairing job", true, true},
	}}
}
