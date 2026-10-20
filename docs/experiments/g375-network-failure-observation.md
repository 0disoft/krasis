# g375: Winsock evidence without inferred firewall causes (#72)

Connection acquisition now unwraps native Windows Winsock errors as well as
portable errno values. Access denied, connection refused, timeout, unreachable,
reset, aborted and cancellation remain separate fixed codes. TLS/authentication
keeps its existing separate failure classification. TCP and TLS/auth duration
observations from #71 also carry a fixed error code.

Every new acquisition observation explicitly records policy and network profile
as unknown: this build has no WFP/profile event provider. Access denied is not
proof of firewall policy; neither are timeout/reset. Active-session termination
records separately classify wrapped TCP socket errors and retain their existing
bounded native code. Non-socket pipe/native failures remain not_observed.
No firewall settings are changed and no allowance is broadened.

Passed focused package tests cover wrapped real Winsock constants across
acquisition and session termination, portable errors, cancellation, unknown
policy, non-socket denial exclusion, fixed-code export validation, existing
serial recovery and LOCAL cleanup. Hardware firewall/VPN changes and signed
release acceptance remain in #6/#7/#16. No specific firewall event or product
is asserted as the cause of the user's recovery state.

The implementation adds optional support/v1 fields and three reconnect reason
codes; old session-end records remain readable. No DB, dependency or runner
changes. g375 packages both #71 and #72. Device results are recorded after
application. No push and no issue comment.

## Paired g375 acceptance and issue disposition

Both PCs run generation 375, revision
`cab7f3c71c911dcd3979d08757a0bff971532919`. Offline verification accepted
27,841,017 bytes. Transfer `c964da68bcc2e4a50f15308627a88de5` reports
UPDATED at that revision. A installer exit was 0 and its installed agent
SHA-256 matches the bundle. Doctor reports healthy/CONNECTED with matching
CLI, UI, agent, installed agent and recovery versions; B is connected.
Frontend tests (85), native build, installer provenance and the eight-file
bundle verification passed, as did the six focused Go package checks.

The 2026-10-20T19:24:14Z/15Z paired collection shows READY input health on
both PCs and complete current agent/control logs. The older control.previous
file is incomplete on both PCs and is not used for current acceptance.
While A was stopped for installation, B correctly logged five-second TCP
connection timeouts, suggested route only, unchanged local path and endpoint,
and unknown policy/profile. After A restarted, B recorded successful TCP
connect (9,708 microseconds) and TLS/authentication (23,423 microseconds), IPv4,
DNS not applicable, selected/suggested NIC equality and route revision 1.
No raw identifiers or addresses are copied into this record. A is the listener,
so absence of its outgoing dial records is expected. Neither current report
contains a fault/session failure/reconnect event after 19:23:45Z.

This short connection window verifies observation and replacement, not a VPN
switch, WFP event, hardware fault matrix or long-term stability. #71 and #72
were closed as completed without comments. Ten issues remain open:
#4, #5, #6, #7, #15, #16, #20, #50, #59, #70. The dependency dashboard was
left untouched. No push, remote branch update or new branch/worktree.

## Resume priority

1. #59: remove remaining wait-capable desktop activity/cursor/observer work
   from low-level callbacks in source_hook_windows.go. g372 already moved
   activation native held-key queries out of physicalMu; do not repeat it.
2. #50: bound lifetime of non-cancelable native injection/guard work while
   preserving partial injection, pressed-state and LOCAL recovery ownership.
3. #70 and #4/#5/#6/#7: gather actual endpoint, hardware and fault-matrix
   evidence; deterministic fixtures alone cannot complete these conditions.
4. #15/#16/#20: complete remaining actual UI, installation and signing gates.

Both implementation tasks began before this round's ten-minute cutoff.
No third task was started. Completed source and acceptance records are local
main commits only. No DB, dependency or runner changes. Product control API
is unchanged; optional support/v1 diagnostic fields were added.

Route API reference:
https://learn.microsoft.com/en-us/windows/win32/api/netioapi/nf-netioapi-getbestroute2
