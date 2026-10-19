# g360: Keep the selected operation and its original native cause together

## Reproduced defect

G359's new source-authority metadata is selected unconditionally ahead of a
more specific native cleanup operation. Joining hook-release/native 170 with
authority-console-token/native 5 then serializes the latter instead. Cleanup
still fences the service, but the diagnostic obscures why cleanup failed.

`TestConcreteCleanupReportCannotBeReplacedByAuthorityDetail` fails against the
actual g359 report_failure.go through a temporary Go overlay in .cache and
passes against the current correction. The retained device baseline is g357;
g359 was packaged, not applied. This is a reproduced local candidate defect,
not evidence of the earlier real source-hook failure's cause.

## Change and validation

Desktop/source failure helpers now return the selected deepest operation with
its own cause. Their existing operation-only functions keep the same behavior.
The report encoder extracts the numeric native error from that selected branch,
instead of the first native error anywhere in a joined error tree. Source
authority details refine generic source/worker containers, but cannot replace a
concrete hook/raw/cursor/desktop cleanup operation. The fixed report layout,
operation vocabulary, failure status and service recovery fence are unchanged.
No raw error text crosses the support boundary.

Desktop and source tests cover a preceding unrelated native error while
retaining the chosen operation's native 170. Report tests cover the same branch
pairing and encode/decode concrete cleanup failures alongside authority errors;
ErrRecovery and the service fence remain. Inputdesktop, wininput, inputworker,
productlog, logindiag and localapi tests pass, as does
`go vet -unsafeptr=false` for those packages. The old-code overlay deliberately
fails the new focused regression; the corrected-source run passes.

Bugfix, desktop-app and error-message-integrity review apply. G360 is reserved
locally, not built or applied. Both PCs retain g357 awaiting its attended movement
comparison. No public API, DB, runner, dependency, repository-hygiene, input
policy, deadline, input replay or permission change; no push, release, CI,
installation, process restart, reboot or live input injection is included.
The original long-term source stall and human-visible movement latency remain
unverified. The new helpers are internal package APIs for diagnostic selection;
they return causes to the existing numeric projection, not to UI/CLI messages.

## Packaged candidate

G360 at source revision `7ab3a4aa77c90cd5bc519ff334cea61b58142a07` now passes
the workspace build, including 85 frontend tests, native assets/UI compilation,
installer provenance, archive roundtrip and eight-file/icon verification.
Offline update verification returns valid/ok and archive size 27895442 bytes.
Independent PowerShell SHA-256 values:

| File | SHA-256 |
| --- | --- |
| krasis.exe | 82c34662734260aacdf1c5de955e515b11a7d07c150cb164245dd5a47984388b |
| krasis-agent.exe | a84aed84c29a38070ecec820653ec7150dc0594cf4b6530e7c37f470cc9a4dbf |
| krasis-cli.exe | 489f45fc6a56c9265db36e92f5904d27bee5e123f79aeb49e357e12265da1edc |
| krasis-install.exe | 600e9a8feef10ae15caee916f063cec9f706cd315ab9002071af4b24ddb1b0d6 |

The earlier "not built" statement describes the source-validation checkpoint;
the candidate is now built but remains unapplied. G357 device state and its
pending attended movement comparison are unchanged. No additional live trial,
service restart, installation, push, release or CI run was performed. Build and
artifact checks do not establish a human-visible movement improvement or
resolution of the original long-term source-hook failure.
