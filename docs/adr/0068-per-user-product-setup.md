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
Before removal changes any files or service, the current registry context must
also expose the owned Installed Apps entry for the receipt's fixed app root.
A missing or foreign entry is a failed preflight, not an idempotent success.
Removal validates saved preferences, startup/recovery registration, both launch
records and the byte-verified Start Menu receipt before removing protected input.
Already-cancelled removal stops before creating product state. Changed shortcuts and
malformed receipts block removal; the actual mutation helpers recheck ownership.
Only launch records for the receipt-listed installation directories are deleted.
Current and previous portable records outside that set are preserved, including
a newer portable launch published after preflight; cleanup rechecks both records
under the same registration lock used by launch publication.
Independent launches retain the Windows system, profile and ProgramData path
variables required by shell/COM services. The bounded environment excludes
unrelated caller values. Both ordinary login/relaunch and job-detached child
creation share this allowlist. Omitting SystemDrive can produce a literal
`%SystemDrive%` cache directory inside the immutable app image; that directory
remains an unexpected file for removal, never an implicitly allowed exception.

Setup and repair apply the saved startup preference. Before publishing another
launch record, they migrate the owned recovery task to the current image while
its previous-image proof is still retained. After publication they move the task
to the new image and register the fixed login CLI. Repeated repair cannot outrun
task ownership history. A foreign or unreadable task blocks this registration;
it is never silently deleted or adopted. A missing settings file keeps the
first-install default, while malformed settings abort installation.
Before staging or replacing installation receipts, setup also checks both launch
history records and the visible Installed Apps and startup registrations. Missing
shell registrations remain repairable; foreign paths, unreadable values and
malformed launch history stop installation without changing application images.
A damaged executable with a valid launch record still permits image repair.
Setup reads recovery-task ownership before requesting UI or agent shutdown,
even when startup is disabled. Missing, disabled or owned stale tasks remain
repairable. On first install, missing launch history permits read-only scheduler
inspection, not skipping it: an existing task without matching ownership blocks
setup. This check never creates, deletes, disables or migrates a task; publication
still rechecks ownership when applying the saved preference.
Startup enable/disable rechecks the fixed command before replacing or deleting
it; a foreign registration is an error rather than a disabled preference.
When repair finds a damaged current image, the owned task is disabled before
publication instead of re-arming that image. The saved startup preference is
applied only to the verified replacement. This does not adopt a foreign task.

Development setup remains unsigned. Production requires a configured Authenticode
signing identity, exact signer thumbprint and timestamp verification, plus the
existing Ed25519 release feed key. Neither trust stores nor security settings are
modified by signing/building. Signer secrets stay in the CI release environment.
The exact signed agent embedded in the protected-input installer is also published
in the application bundle; independent signing timestamps cannot break image pins.
The bundle's signed field is descriptive, never an authentication grant. Official
update authenticity still comes from the verified envelope and exact archive hash.
Signed release bundles use windows-release purpose; manual bundles remain explicit.

The fixed recovery CLI resolves update verification and sending through the current
launch record. The remembered UI hash, bundle generation/revision and copied CLI
bytes must still match before using that directory. Portable CLIs retain their
adjacent bundle; missing or damaged registrations never fall back to another image.
Bundle verification remains offline and SEND still re-packs and validates all bytes.

The first-run screen reuses the pairing flow and explains same-network setup, PIN
registration and screen-edge movement. It does not create peers or request input
installation automatically. Build verification never installs or launches the UI.

Interactive and JSON install/repair both stop the running product and launch the
installed UI. Validated package, settings and registration preflight precede any
shutdown. Before requesting shutdown, setup also parses the existing installation
receipt, rejects downgrade and merged-history overflow, and reads the complete
self-contained setup image. These preflight failures leave the running UI and
connection untouched. Publication uses those already-read bytes, never a second
source-file read after shutdown. Cancelled requests stop before shutdown, and
cancellation during confirmed shutdown releases the idle agent lease without
staging application images. The installer holds the existing startup lease through publication and
retains the idle agent lease after confirmed exit to fence automatic recovery.
Only same-user, same-session UI processes matching the current/previous launch
record and exact image hash can be closed. Retained process handles prevent PID
reuse. For existing Wails images, WM_CLOSE goes only to the verified process's
hidden main-loop window, invoking App.Quit and its normal shutdown callbacks.
WM_CLOSE on the visible window would merely hide the app; WM_QUIT would skip
normal cleanup. The UI also cancels its workers before service shutdown waits.
No process is force-killed, and unknown ownership or an unconfirmed exit blocks
installation. Agent shutdown uses authenticated, instance-bound maintenance and
upgrade-stop requests, preserving saved connection intent and waiting for guard
exit. A 30-second shutdown timeout yields shutdown_failed plus shutdownStage in
JSON; no installation images or registrations are changed by failed shutdown.
The installed UI opens visibly with --installed and uses the existing saved-
connection resume logic, including manual-stop and current trust checks.

Repair selects its immutable image directory during preflight. Missing image
files or changed bytes select a fresh random directory; an unreadable directory
or file blocks repair instead of being classified as damaged. The extra repair
entry must fit the 64-folder receipt limit before stopping the product or staging
any image. Random IDs cannot reuse a receipt entry or adopt an existing folder.
Staging revalidates the chosen image and aborts on a later change rather than
creating an unplanned second directory after shutdown. Existing damaged images
and installation history remain preserved.
