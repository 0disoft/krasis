# Credential UI input probe

This is an attended Windows experiment, not a supported peer-input mode. The regular Krasis agent remains unelevated and still stops a session when it detects protected input.

On device B, with the PIN prompt currently closed, open an elevated PowerShell window and run:

```powershell
Start-Process -FilePath 'C:\path\to\g59\peer-update\krasis-agent.exe' -ArgumentList 'credential-ui-probe' -Verb RunAs
```

Within 60 seconds, trigger the Windows Security PIN prompt on B. The probe sends one Escape press/release only after it confirms `CredentialUIBroker.exe` owns the foreground on the Default desktop. Its result appears in a message box. A `dialog_dismissed_candidate` result means the broker lost foreground after the Escape attempt; confirm visually that the PIN prompt actually closed. `dialog_still_open`, `escape_send_failed`, `dialog_not_observed`, and `not_elevated` do not establish input access. Never enter or send a PIN as part of this experiment.

The probe does not start the resident agent or connect to the peer. A positive result only supports further work on a user-consented elevated destination design; it does not prove that A's keyboard and mouse can control B's prompt. Do not use the probe command for normal app startup.
