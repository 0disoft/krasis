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

## Remaining execution

Release console selection, confirm the existing installer exits, then rerun full
removal and verify registrations and installed images are gone while identity,
trust and diagnostics survive. Reinstall g236, restore its protected-input
component through attended Windows consent and restart via the fixed CLI.
First-run onboarding, reboot behavior, actual A/B input and PIN control were not
tested in this run. The corrected hidden-console removal path still needs an
actual successful invocation; its build alone does not establish that outcome.
