# Application setup smoke: g234 through g236

Local Windows execution on 2026-10-08. No public release or peer update was
performed. Artifacts and captured output are under the ignored
`dist/setup-smoke/g234-20261008-170920` directory.

## Results

| Operation | Observed result |
| --- | --- |
| Original g234 setup install | Exit 1, `install_failed`; own DELETE-capable directory handle conflicts with `os.OpenRoot` |
| g235 actual install and same-version repair | Both exit 0, `installed` |
| Shell registrations | Start Menu targets the fixed recovery CLI with `relaunch`; HKCU Installed Apps reports g235, then g236 |
| g236 actual install | Exit 0, `installed` |
| g236 corrupt-image repair | Exit 0; stages a different folder, matches the distribution SHA-256 and retains the damaged image |
| Identity and trust preservation | All seven pre-existing identity/trust files retain their SHA-256 values |
| Protected-input removal during g235 full removal | Fresh input receipt reports result 0, stage `complete`; service and input directory are absent |
| g235 full application removal | Exit 1, `remove_failed`; child console remained in selection mode past the 150-second exit deadline |
| g236 removal while old installer remains alive | Exit 1, `close_app_required`; installation receipt unchanged |

The protected-input removal receipt is not proof of full application removal.
The old elevated `krasis-install.exe` remained alive in console selection mode
at the final observation. It was neither terminated nor overlapped with another
protected-input installer. The application and recovery registration point to a
verified g236 installation, but the UI and agent have not been restarted and the
protected-input service is absent pending completion of that process.

## Fixes and local verification

- `77b5366`: compatible setup-root handle with protected ACL retained; Windows
  regression fails before the fix and passes for initial creation and repair.
- `fa2a3d4`: app-invoked elevated input installer uses a hidden console, retaining
  Windows consent and process-exit verification.
- Tests and vet passed for appinstall, updatebundle, localapi, productpanel and
  krasis-install in the corresponding change scopes.
- UI 78 tests, native UI builds, update-bundle checks and setup `--verify --json`
  passed for both new artifacts. `git diff --check` passed.

## Follow-up completion: g237 through g240

The user released console selection and the original installer exited. Full
removal exposed a stale recovery task pointing to g231 after repair replaced the
two-record launch history. The original launch receipt and image hash were
validated before the unchanged ownership-checking task script removed that old
task. g236 removal then returned exit 0, `removed`; all receipt-listed folders,
setup image, shortcut and Installed Apps entry were verified absent.

Subsequent corrections:

- `3f929ec`: migrate the owned recovery task before replacing launch history,
  then apply the saved startup preference to the new verified image.
- `f803899`: disable the owned task of a damaged image before repair, rather
  than trying to re-arm an image whose checksum is no longer valid.
- `29c9e73`: require a visible Installed Apps registration for the fixed app
  root before removal changes files or the protected-input service.
- `6cbf5cd`: preserve required Windows environment paths during independent
  launches. Missing SystemDrive/ProgramData was reproduced by a failing native
  child-process regression. Windows shell caches had appeared under a literal
  `%SystemDrive%` directory in the repository and installed g239 image. The
  unexpected app directory correctly blocked removal. The three known cache
  files from each affected directory were relocated to ignored evidence storage
  after confirming their exact ownership paths and absence of reparse points.

g238 real corrupt-image repair succeeded into a fresh folder and its native
recovery task followed the replacement image. g239 independent install, repair,
registered-uninstaller dispatch and full removal also passed without protected
input present. Its later protected-input removal attempt did not complete and
left the service intact; it is not counted as a successful protected removal.

g240's independent lifecycle run passed install, same-version repair, registered
uninstaller dispatch, full removal **with protected input present**, and
reinstallation. The worker observation confirmed receipt-listed folders, fixed
setup, Installed Apps registration and protected-input directory absent before
reinstallation. The setup archive hash matched BUILD.json. Native input
restoration subsequently exited 0 with a fresh installation receipt reporting
result 0, stage `complete`, recovery `not_needed`.

Final observations:

- A UI, CLI and installed protected agent are generation 240, revision
  `6cbf5cdf68746f61836924a30a9c1b255b822adc`; their versions and image paths match.
- Native status is `CONNECTED` to alias B with `inputBackend: PROTECTED`.
  Doctor reports `healthy`, verified installed agent and running matched service.
- All seven original identity/trust file hashes remain unchanged.
- Startup and automatic-update preferences are enabled; the native recovery task
  is enabled and targets the current installed UI. The restored peer selection
  was validated through appsettings and B was connected through the product CLI.
- The app and repository no longer contain literal-drive cache directories.
- The independent same-user registry check reads g240 in both registry views.
  The calling terminal's earlier cached/isolated registry observation showed
  g238; those two observations are retained separately rather than conflated.
- Setup remains unsigned and development-only. B was not updated or reinstalled.

Tests/vet passed for appinstall, appsettings and launchrecord in the corresponding
change scopes; the native Windows environment regression failed before and
passed after its fix. UI 78 tests, native builds, update-bundle checks, setup
`--verify --json` and diff checks passed. Actual first-run onboarding, Windows
reboot, A/B physical input and PIN control were not exercised in this follow-up.
