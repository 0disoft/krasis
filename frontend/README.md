# Krasis control UI — first implementation stage

Svelte 5 desktop UI based on the selected Midnight Workspace reference.
This is an isolated preview, not a replacement for krasis-control.exe yet.
All device state is in memory; only the preview language preference is stored.
No agent endpoint, bearer token, clipboard API, PIN, process or network control is
exposed to the renderer. Pairing actions are explicitly unavailable in this stage.

Korean and English catalogs cover navigation, accessible labels, dialog copy,
state messages and settings. Explicit locale wins over navigator.languages;
unsupported languages fall back to English. No remote font requests are made.

The next stage must supply a typed Go bridge and authoritative state, preserve
tray/single-instance/update behavior, and remove simulated connection controls.
Do not package this preview as the working KVM management app.

Configured verification: `krasis_frontend_install`, `krasis_frontend_check`.
Configured preview: `krasis_frontend_preview` serves only loopback port 4179 and
stops after ten minutes, or when its development-only `/__preview_stop` endpoint
is opened. It does not persist as a background service. See design-qa.md for the
remaining visual verification gate.
Dependency install is scoped to frontend and disables lifecycle scripts.
The selected major lines follow stable updates; package-lock records the build.
