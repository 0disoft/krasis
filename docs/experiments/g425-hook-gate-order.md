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

## Paired rollout and stopping point

Both devices installed g425 revision 8f51c46bf5dfa9ae48914e3a4ab7fbc379e6448c.
B update 28c7fbbf6be7acede9620b05dc773271 reached UPDATED; its installer
completed with result 0 at 11:46:37.9853432Z. A initially returned result 5
(finish/commit/file_delete, Win32 5) while the old g424 UI was running. After
stopping only the path-verified old UI, repair completed with result 0 at
11:48:23.5019542Z. This was a rollout obstruction, not an agent crash proof.

A's UI was subsequently absent and its launch record still referenced g424.
Running g425 app repair followed by relaunch restored it. Doctor at about
11:52Z reports UI RUNNING, launch record VERIFIED, matched g425 UI/agent/
installed revisions, service RUNNING, connection CONNECTED and completed
update-stage cleanup. B's UI was also opened during rollout.

Fresh paired diagnostics collection 58dab39802ff5797ef721b4b8e6980ce was
received at 11:52:31Z/11:52:33Z. Both report inputHealth READY at 11:52:22Z,
service READY and agent_present/CONNECTED recovery observations. B also has
nine reconnect_acquire_network_timeout events at 11:46:54Z through 11:48:43Z,
followed by CONNECTED at 11:49:11Z; these are not evidence of post-rollout
continuous connectivity. Neither side
contains a g425 handoff_profile sample in this collection. Therefore there is
no new physical movement latency comparison or proof that B stutter improved.
The last user comparison was still similar on g423; preserve that unresolved
result rather than treating source regressions or idle health as acceptance.

Issue #59 was closed at 11:49:00Z after checking its original callback/observer/
safe-abort criteria; see results/2026-10-21-issue-59-acceptance.md. Physical
compatibility, lifecycle, signing and endpoint-change requirements remain open.
No new implementation starts in the finishing portion of this bounded round.
Next: collect a real g425 movement/handoff sample and compare source cadence,
transport and receiver injection before selecting another performance change.
