# g304 source command lifetime and latency evidence

## Observations

The user completed the g303 lock/PIN/passkey round trip and reported frequent
stutter. Paired collection `820f1906c6a056ec6c19a83eb6ea7217`, filtered to revision
`d13a4c0cba3e86aaa57d46ed4e5b774f9a990948`, also contains A source heartbeat
failures at 10:22:45 and 10:25:26 UTC on 2026-10-15, followed by reconnects.
Both installed services remain READY. The successful user interaction does not
establish continuous session stability.

A's 10:23:59 runtime window reports queue maximum residence 129054us and
dispatch maximum 764851us. B's 10:24:35 window reports input/dispatch maximum
209618us. These interval maxima neither identify one shared event nor add to an
end-to-end duration. Source command latency was not independently measured.

## Change

`ExecuteSourceDesktop` for the installed interactive source now borrows and
restores a fixed, authenticated Default/Winlogon desktop using RunPresentation.
These commands read GUI state or post to already existing native owners; they
create/retain no windows or hooks on their caller thread. This removes their
per-command disposable-thread termination wait. The actual hook/window owners
still use RunCapture and require pinned kernel-thread retirement before desktop
close. Fixed pre-login source mode retains its existing policy. Authentication,
desktop activity checks and restoration errors are preserved. No command replay,
fallback, new desktop selector or relaxed timeout is introduced.

## Bounded diagnostics

Source command timing uses fixed `installed_source_timing_client_*` codes for
lock, authority, write and response. Heartbeats use a separate
`installed_source_heartbeat_timing_client_*` family. Each holds only the maximum
bucket per 15-second observation window; failure/stop flushes the partial window.
The five buckets are lt1ms, lt10ms, lt100ms, lt1s and ge1s. Counts describe emitted
window maxima, not input/event counts. Omitted stages are unobserved, not zero.
No worker duration is inferred from response time.

Failures additionally emit one fixed phase: context, authority, write, response
or protocol. The phase describes where failure was observed, not its underlying
native cause. Payloads, keys, buttons, PIN values, coordinates, error strings and
identities are excluded. Local source packet framing and peer protocol are unchanged.

The g304 change removes avoidable lifecycle work; its effect on physical-device
stutter and session continuity still requires fresh g304 evidence.
