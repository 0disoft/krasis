# ADR 0068: Per-user application setup and release signing

- Status: Accepted for local implementation; external distribution is not activated.

The self-contained setup stages the same verified eight-file application bundle
under the protected per-user product root's app directory. It accepts no custom
installation/removal paths. Existing images are retained; repair stages a new
verified folder rather than overwriting a running or corrupt image. Windows
Installed Apps and the existing fixed recovery/start-menu entrypoint are registered.
Identity, trust, preferences and diagnostics are preserved on removal. Removal
refuses active/unknown process ownership and unexpected files, and uses only a
strict receipt's fixed child names. An independent temporary worker waits for the
registered uninstaller to exit before deleting it. Protected input retains its
attended, revision-verified administrator removal; the per-user setup is asInvoker.

Development setup remains unsigned. Production requires a configured Authenticode
signing identity, exact signer thumbprint and timestamp verification, plus the
existing Ed25519 release feed key. Neither trust stores nor security settings are
modified by signing/building. Signer secrets stay in the CI release environment.
The exact signed agent embedded in the protected-input installer is also published
in the application bundle; independent signing timestamps cannot break image pins.
The bundle's signed field is descriptive, never an authentication grant. Official
update authenticity still comes from the verified envelope and exact archive hash.
Signed release bundles use windows-release purpose; manual bundles remain explicit.

The first-run screen reuses the pairing flow and explains same-network setup, PIN
registration and screen-edge movement. It does not create peers or request input
installation automatically. Build verification never installs or launches the UI.
