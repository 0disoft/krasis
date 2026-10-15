# g311 source capture and trust maintenance

The g310 paired request `9d731db3403eb4a90b645f87793f5191` contains twelve
matching source/receiver profiles. Source capture activation is 23.804–177.508ms
(median 43.337ms), dominated by local IPC response wait. All twelve receiver
profiles use prepared workers. Source-hook failures and a separate trust
freshness expiry also remain. These observations do not identify native
activation's internal cost or establish a controlled performance comparison.

Changes:

- The window-free source command dispatcher retains one interactive desktop
  scope. Every command rechecks authority and activity; a desktop change retires
  the old binding before acquisition. Native window/hook owners never migrate.
  Cancellation and failed restoration still retire/fence the owner.
- Overlapped pipe cancellation joins the exact kernel operation and preserves
  its completion result. A successful read/write is not rewritten as timeout
  merely because observation ran after the deadline. Truly cancelled IO keeps
  its original cancellation/timeout, and incomplete frames remain failures.
- Heartbeat authentication and the sent request have separate budgets. The
  latter is at most one second, capped by the previous successful request start
  plus the unchanged two-second worker idle timeout. Skipping renews nothing.
- A session trust monitor keeps one fully validated immutable key anchor. It
  still rereads trust, roles, revocation, certificate bytes and ACLs under the
  shared lock on every check. Only parsing/signature work for identical bytes is
  reused. Reconnect creates a new monitor; missing/changed files fail closed.
- Fixed certificate open/ACL/read/close/validation/match timing phases distinguish
  filesystem delays from cryptographic work without exporting certificate data.

Local tests cover completed-IO/deadline races, real pipe cancellation and reuse,
command owner/cancellation/cleanup fencing, heartbeat budgets and unchanged trust
checks after revocation, certificate removal or corruption. Device latency,
idle-entry stability and lock/PIN transitions still require a g311 paired trace.
No peer wire, database, dependency, runner or permission policy change.

## g312 follow-up before device handoff

The g311 unattended post-install observation still failed on A: source reader
timeout at 17:11:38Z and heartbeat response timeout at 17:12:24Z on 2026-10-15.
Native owners joined; the service stayed READY. g311 is not a stability success.

g312 routes an authenticated heartbeat's reply through the serial reader rather
than the native command queue. Other commands must finish their exact ACK before
the reader starts another idle read, so a pipelined heartbeat cannot overtake a
native command and the read timeout cannot cancel work already being processed.
The existing command deadlines, lease expiration, authority checks, writer
sequence, response authentication and native cleanup gates remain in force.
Tests hold a pending activation, prove no next read/reply occurs before its ACK,
and verify heartbeat write failures, cancellation and stop do not deadlock.
