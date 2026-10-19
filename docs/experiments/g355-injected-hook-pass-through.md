# g355: Avoid physical desktop work for ignored injected hook events

g354's attended movement trial clearly improved perceived stutter, but retained
receiver ordinary dispatch tails of 78778us and 103852us. The long movement tag's
worst operation includes a 27613us native submission and 72882us until the client
observes its first ACK bytes. These are overlapping local spans, not additive
or proof that low-level hook work accounts for the whole delay.

Static inspection finds that source low-level callbacks perform
sourceLoginDesktopCheck before excluding already-ignored injected events. In
protected capture this enters Scope.Do and its current desktop/authority checks.
The Windows low-level hook executes by messaging its installing thread; work
there is therefore relevant to the synchronous native input path. The exact
contribution to the retained tail is unmeasured.

The bounded change bypasses physical desktop work only for keyboard injection
already excluded by the mapper and tagged non-wheel mouse events already excluded
from physical routing. On the owner thread, it still invalidates pending passive
presentation, propagates earlier cleanup failures and updates injected mouse
position. If a shield is visible, current-desktop validation still precedes its
synchronous native hide. No injection, physical suppression, boundary intent or
lease authority is added to this branch. Actual physical events are unchanged.

Tagged wheels retain the original capture-window suppression; untagged synthesized
wheels retain precision-touchpad routing. Tags are not authentication and are not
used to admit input. The injection worker independently retains every request's
authority, desktop, sequence, binding, deadline and matching ACK rules.

Callbacks are extracted into one owner-local factory for direct tests; native
registration remains the same singleton keyboard/mouse trampolines and thread-ID
owner map. No new timer, callback slot, per-event task or process is introduced.
Preparation lifetime, lease timeout, transport and g354 ACK policy are unchanged.

Focused tests call the actual owner callbacks with injected/tagged events while
the physical gate is held, verify no boundary emission or desktop check, and
verify pointer/presentation invalidation. Other tests preserve physical checking,
touchpad-wheel routing, owned tagged-wheel suppression, visible-shield validation,
restoration and retained cleanup failures. Device benefit remains unverified
until both PCs run this build and the attended movement trial is collected.

First-entry preparation remains a separate unresolved cost: the g354 long tag
had no prepared candidate and about 232ms source request-to-grant. This patch
does not extend warm-child lifetime or claim to fix cold entry.

Primary reference: [Microsoft LowLevelMouseProc execution model](https://learn.microsoft.com/en-us/windows/win32/winmsg/lowlevelmouseproc).

## Local validation and paired rollout

Source revision `f0c9dc3a27bfc19646351ddb9226cdfd78980a8d`, generation 355.
Wininput, inputdesktop, inputworker and localapi tests pass, as do desktop-tagged
workspacehost tests, all 85 frontend tests, native UI build, installer provenance,
update package roundtrip, file/icon verification and offline update verification.
`go vet -unsafeptr=false` passes for the four related packages. Unfiltered vet
reports Win32 callback LPARAM/uintptr conversions (including the extracted
callbacks and pre-existing capture/shadow/window callbacks); it is not a clean
unfiltered-vet result. No race run was added for this owner-thread-only change.

Independent PowerShell SHA-256 values:

| File | SHA-256 |
| --- | --- |
| krasis.exe | 5e03de16cba3148eaf98a86a7e99192cfa639def807ff9e7e8129c22e3c6dded |
| krasis-agent.exe | e6a43a9c119229006d66ef40f7451f6a7de9b651e8220bdd749b4980f6bfce7f |
| krasis-cli.exe | 6fd6e6d8a3af770c0b37a6951094851b32133fb7683010aa0f4e215f8cd1ffc2 |
| krasis-install.exe | 93a8f684937dc6253e5e72fb6fc3486448ff979dc4bdd86f2b3fa75f781d82e1 |

B update `be1b170cf1830d052ebc167836f62267` reached UPDATED at this revision.
A then used ordinary UI replacement and automatic protected-input refresh,
followed by connecting its existing B profile. No reboot or manual service start
was used. A doctor is healthy/CONNECTED/PROTECTED/CONFIGURED with matching UI,
agent and protected installation. B is observed g355 ONLINE/IDLE at this revision.

Paired collection `a2486eebde9da728e99e00d1ba97725a`, both supports created at
2026-10-19 13:48:21Z, is RECEIVED. Both supports have install result 0/complete/
recovery not_needed, service READY and input-health READY; new-revision events
retain no session-end trigger or fault. Both retain g354's accepted ACK option.
Old control.previous.jsonl is incomplete on both. Automatic installer process
exit/stdout were not captured independently of its receipt. This short baseline
does not prove the performance hypothesis or long-term stability.

The next attended comparison is the same idle/continuous movement/three-return
trial as g354. No new PIN trial is requested for this performance step. No push,
public release, CI run, API/DB or runner change was made. ADR 0066 records the
ignored-event boundary; bugfix and desktop-app procedures plus the incident
evidence review were used.

## Later retained observation, 13:54:47Z

Collection `34a40768a602457db87d5c42ecb25d65` is RECEIVED. Both g355 supports
still show service/input health READY and no retained new-revision session-end
trigger or fault. Three matched handoff profiles are present on each side;
ordinary dispatch maxima are A 2015us and B 21282us. This is a much smaller,
uncontrolled sample than the g354 attended movement trial, so it does not yet
establish a movement-stutter improvement. Both supports retain an incomplete
older control.previous.jsonl. A doctor remains healthy/CONNECTED/PROTECTED.
The requested g355 user comparison is still pending. A separate local g356
preparation-watcher patch has been committed without changing either live PC.

## Continued retained observation, 14:04:12Z

Collection `a72c61cbce7ac4472698a9cc587fd908` is RECEIVED (A support created
14:04:12Z, B 14:04:11Z). Exact g355-revision records show service/input READY on
both sides, no retained fault/session-end trigger, and 10 matching handoff tags.
All profiles end ok; all 10 B startup records select a prepared worker. Maximum
ordinary source dispatch is 8236us and receiver dispatch 21282us. Thus the earlier
79..104ms receiver tail is not reproduced in this retained sample. The larger
sample still lacks confirmation of the exact requested continuous-movement trial
and its perceived result; it cannot establish an effect size or zero stutter.
Both supports flag the older control.previous.jsonl as incomplete. A doctor
remains healthy/CONNECTED/PROTECTED, and g356 is only built locally, not applied.

## Attended feedback and paired observation, 14:07:33Z

The user completed movement and returns, reporting possibly more stutter than
g354 but explicitly noting concurrent work on A. Treat this as inconclusive
perceived regression, not a controlled g354/g355 comparison.

Collection `85698d6530b363c3fed5c4c440830853` has both supports created at
2026-10-19 14:07:33Z. Exact g355 revision records contain 22 matched handoff tags,
all ending ok, with no retained fault or session-end trigger. Both services and
input-health records are READY. B's eight reconnect_acquire_network_timeout
records are at 13:46:38..13:47:48Z, during rollout, before the movement window;
they must not be counted as attended-trial disconnects. Both exports flag the
older control.previous.jsonl as incomplete.

The last 12 matched handoffs, around 14:06:08..14:06:51Z, have ordinary dispatch
maxima A 16286us and B 9584us. B reuses preparation in 11 of these 12; the
998-frame receiver profile has preparationDecision 3 with no prepared worker,
while its paired A request-to-grant is 310024us. This supports keeping cold-entry
preparation as a separate concern. Across all 22 retained profiles the ordinary
dispatch maxima are A 16286us and B 21282us.

A's 14:06:12Z runtime interval retains timer lateness 509801us; the following
interval retains queue residence 244128us and dispatch maximum 300692us, with
that slow dispatch classified control. B's final interval maximum 157149us is
also classified control. These maxima are separate observations, not additive,
not ordinary movement dispatch measurements and not proof of another process's
CPU use or a GC cause. Runtime histograms do not time-align an individual pause
to the user's perceived stutter. Host load was not independently sampled.

Current A doctor confirms healthy/CONNECTED/PROTECTED/CONFIGURED. g356 offline
bundle verification again passes at the recorded revision and size. Proceed
with the separately validated preparation-reader cancellation change, retaining
this g355 evidence; a quiet-host attended comparison remains necessary. No
additional source change, rebuild, public release or CI run is needed here.
