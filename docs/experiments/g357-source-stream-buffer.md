# g357: Bounded source stream read-ahead

## Evidence and candidate

g356's attended trial is acceptable to the user, with 12 successful paired
handoffs, but retains A queue residence 40537us and control dispatch 145006us.
The first source activation has CaptureIPCResponse 100814us and the long movement
handoff later records SourceDeactivate 145005us. These are different operations;
they cannot be summed or treated as proof of one bottleneck. Later retained
collection `4f8abda61fc1efe3ec41e9795165b2e3` still has no fault or session-end
trigger and shows additional scheduling/control tails. Keep the working g356
baseline while testing changes locally.

Code inspection finds the source agent reads every fixed 64-byte packet directly
from the native pipe. Each native read creates and closes a completion event and
pins a new OVERLAPPED, including when multiple packets already occupy one native
pipe write. Advisory boundaries are already written in batches of up to 16;
actions and exact command replies share that ordered stream. This is avoidable
per-packet work, not proof it caused the user's earlier stutter or IPC tails.

The candidate reader has one 1024-byte buffer owned by one source client after
its unbuffered startup handshake. It never adds a goroutine, timer, input queue,
frame replay, authority cache or write batching. It requests at most one existing
advisory batch and returns a single packet immediately. All decoding, binding,
consecutive sequence, epoch and reply checks remain in their existing order.
Close still belongs to the channel owner; no buffer survives a replacement client.

Windows byte-mode pipes return available bytes and do not wait to fill the
requested buffer. See [Microsoft's named pipe modes](https://learn.microsoft.com/en-us/windows/win32/ipc/named-pipe-type-read-and-wait-modes)
and [ReadFile completion rules](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-readfile).
The native single-ACK test additionally verifies this property on this host.

## Local comparison before product adoption

`BenchmarkNativeSourcePacketStream` uses the same native pair, encoder, decoder,
packets and reverse-direction request token on both paths. The token bounds
producer read-ahead; each benchmark operation reads one whole batch. Three
one-second runs on Windows/amd64, AMD Ryzen 5 7430U:

| 16 packets per operation | direct | buffered candidate |
| --- | --- | --- |
| ns/op | 157553 / 157870 / 160250 | 56508 / 57732 / 56089 |
| native reads/op | 16 | 1 |
| allocations/op | 35 | 20 |
| allocated bytes/op | 1632 | 1152 |

Sequential single-packet subtests varied under host load, so they do not establish
regression. `BenchmarkNativeSourcePacketSinglePaired` alternates direct/buffered
order inside each run on the same connection. Three two-second runs:

| run | direct ns/request | buffered ns/request |
| --- | --- | --- |
| 1 | 51412 | 50867 |
| 2 | 50888 | 51464 |
| 3 | 55036 | 53081 |

This demonstrates amortized native reads under a burst, without a consistent
single-request slowdown in the paired harness. It does not measure cursor-display
latency or prove a human-visible benefit. Both paths allocate the same five
objects/192 bytes per single packet in the original harness.

Native tests pass for ordered batch delivery in one bounded read, an individual
ACK with no follow-up bytes, truncated-frame failure, and closing an outstanding
read. The candidate is not yet connected to the product source client at this
benchmark checkpoint. Production source, generation and both live devices remain
g356. No push, public release, CI, API, DB or runner change was made.

## Product adoption

After the benchmark checkpoint, source_client_windows.go uses this reader only
inside its existing serial packet goroutine, after the startup handshake and
before the unchanged sourceStart exchange. Worker command readers and every
other pipe user remain unbuffered. Generation 357 reserves a distinct artifact
for paired observation. The wire format, frame validation and error handling
are unchanged. This increases per-source read-ahead by at most 1024 bytes; there
is no unbounded backlog, new lifetime owner or change to writer serialization.
Native input is never injected by these transport tests.

Inputworker, localapi, agent and residentagent package tests pass, including all
native source-reader, reply, retirement, authority and cancellation tests.
`go vet -unsafeptr=false` passes for these four packages; the existing Win32
uintptr callback conversions are not claimed clean under unfiltered vet. The
focused native reader tests and both benchmarks also pass. No race run was
added; the new reader is local to the already serial source goroutine and is
never shared. Bugfix, desktop-app and memory-lifetime review procedures were
applied, including the performance/security checklists. Device benefit remains
unverified until a separately packaged build is applied and observed.
