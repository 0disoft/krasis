# Midnight Workspace preview QA

final result: passed

This result approves the isolated visual preview only, not native integration.
Earlier blocked observations below are retained as comparison history and are
superseded by the final Chrome pass at the end of this report.

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

## Chrome fallback and refinement, 2026-09-17

The in-app browser again failed exact-size capture. A separate Chrome verification
tab successfully rendered at 1487 x 1058 CSS pixels and DPR approximately 1. Source
and implementation were emitted together in the same comparison input. Captures
remain inline evidence because the browser API returns no persistent image path.

Initial P2 findings: excessively thick monitor outlines, distracting dot texture,
and an empty feedback row displacing the footer. Replaced the large monitor icon
with generated source-matched artwork, reduced texture opacity from .08 to .025,
and removed empty feedback from layout. A post-fix capture verified loaded monitor
images (1536 x 1024), fine outlines and stand, restored utility/footer placement.
A stronger peer tint trial colored the entire image rectangle and was reverted;
the peer label now supplies additional violet emphasis.

A 640 x 900 narrow-window capture showed stacked utilities, readable labels and
no horizontal overflow (document width 625 within viewport 640). The Chrome
warning/error log was empty. Native integration remains a separate stage.

Follow-up P3: optimize the 1.15 MB monitor PNG before packaging. Slight differences
in native font antialiasing, monitor stand shape and border emphasis are acceptable
for the preview; no fake current agent generation or latest-version status is used.

## Final Chrome pass

The final revision was captured at verified 1487 x 1058 CSS pixels and DPR 1,
with both monitor images loaded. Source and implementation were shown in the same
comparison input. A focused 900 x 290 capture of the monitor region confirmed
readable live labels, fine borders and no thick-outline regression. Typography,
layout rhythm, charcoal/violet/green tokens, artwork quality and intentional copy
differences were reviewed. No remaining P0/P1/P2 issue blocks the isolated preview.
The source's brighter peer border remains P3 visual polish, not a missing state:
the implementation uses a subtle peer tint and violet B label.

Final interaction spot-checks: disconnect enables placement, swap updates side
labels, English switch localizes controls, registration dialog opens/closes with
unavailable native actions disabled, and FAULTED disables placement and shows the
recovery action. Browser warning/error log is empty. The 640 x 900 capture above
is the narrow-window check; minor last-revision label color does not change layout.

## Remaining native integration gate

Go bridge, tray, real pairing, live agent state and Windows installation remain
unimplemented in this frontend and unchanged in the native app. No native tests
or two-PC integration tests were run for this frontend-only change. The configured
frontend build/catalog tests and scoped mustflow check passed. No public API or DB
contract changed; source-of-truth ADR 0060 still limits this to an isolated preview.
