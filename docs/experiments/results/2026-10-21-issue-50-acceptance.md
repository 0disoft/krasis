# Issue 50 acceptance against its actual completion criteria

Source: live GitHub issue #50 read 2026-10-21. This is the local-main
implementation/verification record; code has not been pushed.

| Original criterion | Inspected and executed evidence |
| --- | --- |
| Distinguish slow injection from network delay in records. | Existing frame read, dispatch coordinator, injection, worker apply and ACK-read timings; peer progress/dispatch tests and agent injection-certainty tests pass in the full g424 peer/agent suites. Paired g423 collection 2474b2da442e3d9162199aa7980c898f separately records receiver client authority/ACK wait, native-child exchange, and source TCP/write spans. These are not summed into physical latency. |
| Cleanup cannot remain blocked indefinitely. | TestIsolatedWorkerRecoversHeldInputAfterNativeChildStall uses a real Windows child that ignores cancellation, requires exact child exit before independent key-up and a bounded result. TestJournalRecoveryChildRetiresEvenWhenNativeReleaseIgnoresCancellation also stalls the separate release child and requires its exact retirement and a retained remaining-input failure. Both pass on the current source, together with incomplete-retirement rejection, in 4.309s. |
| On partial/failed injection, pressed-state and LOCAL recovery agree. | Full application tests pass: unapplied presses roll back, partial/unknown presses retain their ledger, late cancelled success uses fresh cleanup, failed release remains FAULT with its token, confirmed release clears the token and converges LOCAL. New pointer-batch failure regression also recovers the held key. Full peer tests retain stalled-input deadline and cancelled-stream held-input recovery. |

The earlier open-issue audit added physical Winlogon/held-input proof as a
prerequisite for #50. The issue's actual completion criteria above do not
require the entire attended native/device matrix. That larger acceptance
remains in #6/#7/#20 and is not claimed here. Closing #50 does not mean that
all B stutter, native device compatibility, reconnect, signing or beta issues
are resolved. Source capture/service failures and physical performance remain
separate open work.

Current verified local implementation commits include d8fe782 and 7185702,
with unchanged native isolation/recovery code tested at those inputs. No
acceptance criterion was removed from the issue; no issue comment or push.
