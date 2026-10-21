# g425: do not hold the physical hook gate while waiting for state

Live issue #59 requires removing callback wait causes, bounded/nonblocking
observation, normal idle handling and safe termination with a cause code when
raw input and hooks disagree. B's g424 input-service repair is still pending;
this is a separately reproduced local source-capture defect, not a claim that
its installation failure or physical stutter has been fixed.

SourceSession.Activate's final transaction, UseExternalPhysicalCapture,
captureRefreshPending and PauseCapture all acquired physicalMu before waiting
for stateMu. A stalled state transaction therefore also stalled physical
callbacks, even though Offer and abort themselves no longer waited for state.
All four two-lock operations now use stateMu then physicalMu. Native pressed
inspection remains outside both locks; its physical revision/held-input
recheck still rejects stale grants. The physical path retains its one short
ledger/queue gate and never needs stateMu. No deadline, desktop check,
suppression policy, pressed-state record or capture-owner retirement is removed.

The new regression retains a state transaction, starts each of the four
operations and checks that a physical key-down remains locally deliverable,
recorded and ineligible for a stale grant. With a Go overlay using the original
HEAD files, all four cases fail at their 500ms callback bound (2.153s total).
With the fix the full wininput/agent/inputworker suites pass in
3.013s/1.919s/25.348s, including blocked observer isolation, idle/short raw
bursts, sustained raw/hook mismatch, nested callback deadlines, native-query
isolation, generation fences and incomplete native-retirement rejection.

The first test harness incorrectly used synctest.Wait around an ordinary
mutex wait, which Go does not regard as durable; both attempted runs timed out.
The corrected harness uses real scheduling and a bounded callback probe.
Only the corrected pre-fix overlay failures and final successful suites are
acceptance evidence. No failed or unexecuted check is recorded as passing.

The physical/device matrix and B performance comparison remain separate.
No public API/wire schema, DB, dependency, runner, signing or firewall change.
No push or issue comment. Build/application results follow separately.
