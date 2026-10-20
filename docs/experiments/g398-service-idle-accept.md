# g398: retain the service accept across idle periods

The managed service loop retained its named-pipe instance, but acceptAgentNamed
created a fresh five-second context before waiting for a client. An idle service
therefore cancelled its pending ConnectNamedPipe, closed its loan, backed off
and reset the instance every five seconds. Client arrival could overlap that
retirement/reset window even though the endpoint name and service stayed live.
This is a code-backed reconnect risk, not proof of the earlier device EOF cause.

Service-owned input/source accepts now inherit the service lifetime instead.
Service stop and any enclosing deadline still cancel the exact pending I/O.
Single-use AcceptAgent keeps its five-second bound. Authentication still runs
before lease consumption, and receiveAgentRequest retains its separate bounded
read deadline. No idle wait grants input, lease authority or native readiness.
Worker bootstrap/control deadlines and existing serial reset/retirement fences
are unchanged. This removes periodic idle accept turnover rather than retrying
input or treating an EOF as success.

Inputworker's full suite and vet pass. A real Windows named-pipe regression test
keeps one accept pending beyond the former five-second timeout, then connects
and exchanges bytes on that same loan, retires it and confirms that cancellation
interrupts the next pending accept. The old idle deadline would terminate the
first accept before the client exists. Virtual-time tests separately retain the
single-use deadline, service cancellation and shorter enclosing deadline.

This does not reproduce the original first mixed-build startup EOF or complete
all #50/#59 conditions. g397's missing-report evidence remains available for a
future failure; known native cleanup failures retain their recovery fence.
No public API, DB, dependency, runner, firewall, signing, branch/worktree, issue
comment or push changes. Paired application is recorded after packaging. This
was the second implementation unit, started before the ten-minute cutoff.
