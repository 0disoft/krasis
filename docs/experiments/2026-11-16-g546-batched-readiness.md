# g546: batch the fixed readiness body

## One selected target

`inputworker.readStartupReady` read the KRD9 envelope through nine `io.ReadFull`
calls: header, stage vector, load, prepared age, decision, preparation wait,
service, native children and image hash. On the Windows product pipe, each
available chunk invokes a new `ReadFile`/overlapped completion with its own
pinning and lifetime bookkeeping. `readInputLeaseReady` uses this same parser.

The g545 physical trial retained eleven successful pairs, with B readiness
median/max 25.395/36.337ms. This identifies a relevant entry path, but does not
attribute that whole wait to the decoder. Authentication and native startup
remain separate work and are unchanged.

## Change

After validating the eight-byte header, select the existing version's fixed
size and read exactly its body into one bounded buffer (maximum body 176 bytes,
envelope 184 bytes). Decode the same fields using slices. No read-ahead into
the following input/report frame, new protocol, cached permission, retry, altered
timeout or authentication bypass is introduced. KRD1 remains one header read;
KRD2 remains two reads; KRD3–KRD9 remove their per-extension I/O calls.
Fragmented streams can still require multiple underlying reads, as required by
`io.ReadFull`; the two-read result applies when the two requested parts are
available in full.

Reserved bytes and numerical diagnostic validation retain their rules. Invalid
numbers discard diagnostics without changing valid readiness. Native read
errors remain wrapped with ErrProtocol. An EOF at a former field boundary
remains EOF; an EOF inside a field remains UnexpectedEOF. A malformed/truncated
body can now be consumed up to its fixed envelope boundary before rejection,
under the caller's unchanged deadline. It can never produce readiness.

## Regression and measurements

The new test failed against the old source with `Read calls=9, want 2`, then
passed after batching, for both direct parsing and the product lease wrapper.
Tests cover KRD1–KRD9, all 720 truncated lengths across those envelope sizes,
one-byte fragmentation, following-frame preservation, and an injected I/O cause.
Existing reserved-byte, invalid-numeric, authentication and startup-retirement
tests remain applicable. The existing fuzz harness now seeds KRD9 and uses the
actual maximum envelope bound instead of its older KRD7 limit.

Both benchmark variants use the identical test and runtime overlay. The before
overlay replaces only startup_ready.go with the committed pre-change source;
the after run uses the edited source. Two 250ms repetitions on Windows amd64:

| Decoder through production lease wrapper | Before | After |
| --- | --- | --- |
| Available-buffer Read calls | 9 | 2 |
| Available-buffer allocations | 16 | 9 |
| Available-buffer bytes allocated | 560 | 544 |
| Real native pipe Read calls | 9 | 2 |
| Real native pipe allocations | 24 | 10 |
| Real native pipe bytes allocated | 808 | 568–569 |
| Real native pipe time | 57.075–197.050us | 33.325–39.199us |

The deterministic improvement is seven fewer pipe reads (77.8%) and fourteen
fewer native-path allocations (58.3%) per KRD9 envelope. Timing samples are
affected by host scheduling; they do not establish a controlled speed ratio
or a reduction in physical crossing latency. CPU/GC scheduling and focus waits
are not addressed by this change.

Full inputworker and handoff tests, focused race tests and vet (with the
repository's existing unsafeptr exclusion) passed. Linux amd64 cross-build
passed after fetching the existing go.mod/go.sum-pinned X11 dependency that
was missing from the local offline cache. No dependency version changed.
Cross-build is not native Linux testing. Paired Windows installation and fresh
physical crossings are separate rollout/measurement stages.
