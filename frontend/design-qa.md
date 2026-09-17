# Midnight Workspace preview QA

final result: blocked

## Visual truth and evidence

- Source: user-selected `exec-b1f2c325-9869-4650-beac-352ed2d15061.png`
  from Codex generated_images/019f4b8c-7a39-70d2-80a5-39fcaa56a200.
- Reference dimensions: 1487 x 1058 pixels, connected Korean desktop state.
- Implementation: http://127.0.0.1:4179, in-app browser tab 1.
- Browser screenshots were inspected inline in this task on 2026-09-17;
  no persistent screenshot file was returned by the capture API.
- An initial reference and implementation capture were presented together.
  Same-size comparison was not achieved: requesting 1487 x 1058 produced
  a reported 1144 x 814 CSS viewport with DPR 1.3. Later viewport and CDP
  overrides produced inconsistent DPR and clipped captures. Overrides were reset.
  These captures cannot prove pixel fidelity or minimum-width behavior.

## Interaction checks

Observed in the browser: Korean-to-English switch, localized accessible labels,
language persistence on reload, disconnect unlocking placement, left/right swap,
clipboard switch, Devices and Settings navigation, pairing dialog and close,
and FAULTED scenario with recovery message. Preview actions have no agent side
effects. PIN creation/discovery are explicitly disabled pending integration.
Browser warning/error log was empty in the inspected run.

## Comparison history and required fidelity surfaces

- Typography: system Segoe UI/Malgun Gothic with separate heading/body scales;
  final exact-size comparison remains blocked.
- Spacing: retained top navigation, central two-device arrangement and three
  utility columns. The early monitor icon was too heavy; switched from regular
  to the library thin asset, widened its display and reduced small-screen labels.
  Post-fix same-size evidence is still required before passing this gate.
- Colors: charcoal panel, violet peer/selection, green connection and amber
  fault palette. Native caption buttons intentionally omitted in browser preview.
- Assets: existing Krasis ICO and Phosphor library icons; no invented logos.
  Added a low-contrast library dot motif. ICO replaced an unnecessarily large
  source PNG (66 kB instead of 808 kB in the build).
- Copy: text-only clipboard corrected; version status is explicitly not checked;
  no invented live version or successful update. Preview notice and scenario
  selector intentionally differ from the production mock.

## Remaining gate

Capture the final build at a verified desktop CSS viewport and a supported narrow
window size, save evidence, and compare against the selected source. Do not call
this production-ready or visually approved until that comparison passes.
Go bridge, tray, real pairing, live agent state and Windows installation are
out of scope for this first frontend unit and remain unchanged.
