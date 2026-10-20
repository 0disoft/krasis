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

## Paired application

Both PCs applied g398, revision ab113d6d01a9d555c003eb12a767441376969045.
UI 85 tests, native builds, installer provenance and eight-file package checks
pass; verified archive is 28,121,709 bytes. B receipt
4830685df3386ecdfb9270c3eafe153b reports UPDATED at the same revision.

A's first installer exited 4 at 23:44:48Z: replace/file_move, Win32 5, previous
installation restored. The g397 installed-agent hash and READY restored service
were verified; no agent processes remained. The original receipt is retained in
the local-only .cache/g398-first-install-restored.json. One subsequent install
exited 0, stage complete, with no recovery required; installed agent SHA-256
matches g398. The exact reason for the first access denial is not established.
No restoration receipt was treated as successful g398 application.

Before replacing B, collection 417e5d540f16a509d7fe26c2b6175fa5 preserved its
g397 history and the first A g398/B g397 connection. Both source captures were
ready, without the prior initial source failure in these logs. B's later update
ended A's session; A was explicitly reconnected without another installation
or Disconnect command. Collection e777d133c00359f5858113d9d511ed71 at
23:47:46/48Z records READY services, both CONNECTED/protected capture ready,
and no g398 startup-failure or fault event. A doctor is healthy and CLI, UI,
resident and installed builds match. This is a short observation, not proof
that the historical startup EOF has a single cause or is permanently resolved.

The installed native cursor smoke from g396 was reused because this unit changes
service accept lifetime rather than injection. The real native named-pipe test
directly covers the new idle-accept behavior. Physical input/held-input, lock,
sleep, endpoint change, home-pair, accessibility, soak and signed release gates
were not run. All ten issues retain their unfinished acceptance conditions.

Next priorities: use g397's preserved stage evidence if startup EOF recurs;
finish #50 recovery-side synchronous native checks, then #59 callback work and
#70's actual endpoint trial. No third implementation unit was started; both
completed changes and application evidence stay on local main with no push.
