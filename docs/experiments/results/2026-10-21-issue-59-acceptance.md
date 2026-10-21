# Issue 59 acceptance on local main

The live original issue asks for callback wait causes to be removed, bounded
observation that cannot cause timeout, normal idle handling, and safe abort
with a reason for actual raw/hook progress disagreement. It does not require
the complete physical compatibility/Winlogon matrix or published CI; those
remain separate issues. Code below is local and has not been pushed.

| Completion criterion | Inspected and executed evidence |
| --- | --- |
| Remove callback wait causes. | Optional pipeline/queue/presentation observations use bounded nonblocking relays; timing samples reserve completion capacity and have a process-wide stuck-consumer limit. Native pressed inspection executes outside physical/state gates with revision validation. g425 fixes the remaining state-wait/physical-gate inversion in all four operations requiring both locks. A regression against original source fails all four callback probes, then the full fixed wininput suite passes. Required current-desktop checks, first-input cursor handling and CallNextHook remain; this is not a promise that Windows scheduling or every native syscall has zero latency. |
| Observation cannot itself cause hook timeout. | Blocked timing and pipeline consumers, 10000-sample overflow, bounded replacement consumers, zero-allocation callback relay and observer panic regressions pass in the full suite. Watchdog atomics are independent of optional delivery. |
| Input absence stays normal; real disagreement safely aborts with a cause. | Idle/short raw bursts are ignored, sustained physical raw without mouse-hook progress and stalled/nested callbacks yield distinct hook-mouse-progress or hook-callback-stalled codes. Injected mouse traffic cannot renew physical progress. Terminal fencing is atomic and does not wait on stateMu; unjoined or failed native cleanup cannot become a successful retirement proof. Source owner shutdown joins the three native owners before releasing its session lease; a failed join keeps the owner fenced. Existing inputworker failure/report tests retain these fixed codes. |

Executed after the lock-order fix: full wininput 3.013s, agent 1.919s and
inputworker 25.348s. Packaging also passes 85 UI tests, native/installer builds,
archive smoke (6.63s) and eight-file verification. The corrected harness's
original-source overlay failures are 2.153s; the initial invalid synctest
harness timeouts are explicitly excluded from passing evidence.

Source commit 8f51c46. Device rollout is recorded separately in
../g425-hook-gate-order.md. This closure does not claim that the user's B
stutter, source service recovery faults, physical matrix or signing are fixed.
No issue comment, push, certificate/firewall change or acceptance rewrite.
