# Credential UI input probe

This is an attended Windows experiment, not a supported peer-input mode. The regular Krasis agent remains unelevated and still stops a session when it detects protected input.

If A still runs an older Krasis UI, quit that management UI from its tray menu while leaving the agent running. Start `dist/workspace/g59/krasis.exe` on A and confirm A's title shows `g59`; the single-instance handler otherwise reopens the older UI. Then use **기기 업데이트** to send g59 to B. Confirm B's title shows `g59` before testing. The update transfers the agent too; no manual executable copy is needed.

On B, with the PIN prompt currently closed, open PowerShell and run:

```powershell
$agentPath = Get-CimInstance Win32_Process -Filter "Name='krasis-agent.exe'" | Select-Object -First 1 -ExpandProperty ExecutablePath
if (-not $agentPath) { throw 'Krasis agent is not running on B' }
Start-Process -FilePath $agentPath -ArgumentList 'credential-ui-probe' -Verb RunAs
```

Within 60 seconds, trigger the Windows Security PIN prompt on B. The probe sends one Escape press/release only after it confirms `CredentialUIBroker.exe` owns the foreground on the Default desktop. Its result appears in a message box. A `dialog_dismissed_candidate` result means the broker lost foreground after the Escape attempt; confirm visually that the PIN prompt actually closed. `dialog_still_open`, `escape_send_failed`, `dialog_not_observed`, and `not_elevated` do not establish input access. Never enter or send a PIN as part of this experiment.

The probe does not start the resident agent or connect to the peer. A positive result only supports further work on a user-consented elevated destination design; it does not prove that A's keyboard and mouse can control B's prompt. Do not use the probe command for normal app startup.
