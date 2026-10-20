# Selected-route acquisition diagnostics (#71)

TCP connect and TLS/authentication now produce separate duration observations.
Product profiles contain literal IP endpoints, so those records explicitly mark
DNS as not applicable and distinguish IPv4/IPv6. The socket's local address is
matched to the current interface inventory; ambiguous matches remain unknown.
Windows GetBestRoute2 is sampled after the attempt to compare the current
suggested NIC with the socket NIC. A difference is an observation, not proof
that the OS selected an incorrect route. Failed attempts have a suggested route
only; unavailable lookups remain unknown.

The route revision compares interface LUID/index, source, destination prefix,
next hop and metric, excluding volatile age/lifetime fields. Local path change
and endpoint change are separate fields. Interface identifiers are HMAC
pseudonyms scoped to one connection monitor. Raw indices, addresses, VPN names,
route digests and errors are never persisted or exported.

Observation uses a capacity-16 nonblocking queue. A single process-wide native
probe has a one-second caller deadline; late results cannot reach another
monitor. It owns no socket and cannot start dials. The existing serial
supervisor reacquires with a new session/lease and its capacity-one network
wake; no interface is pinned and no parallel retry path is introduced.

Passed: netobserve, agent, productlog and residentagent package tests, including
native Windows IPv4/IPv6 loopback route lookup, TCP versus TLS failure stages,
local versus remote change fixtures, unavailable/stuck query behavior, queue
saturation, pseudonym isolation and sanitized export rejection. Existing serial
retry tests continue passing. Actual VPN/NIC switching remains part of the
physical compatibility matrix (#7), not a claim made from these fixtures.

No dependency, DB, runner, trust or firewall rule changes. This adds optional
validated support/v1 fields; old records remain readable. Device application
is recorded separately after packaging. No remote push or issue comment.
