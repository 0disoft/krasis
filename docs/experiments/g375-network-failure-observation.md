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
