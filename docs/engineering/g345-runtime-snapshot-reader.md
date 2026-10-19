# g345: preserve a new trace while the previous snapshot is being read

Paired g344 request `d3f3ca0842011d7f31dc1717a63a7c9c` contains two successful
source exchanges of 109,908 and 40,463 us. Receiver pipe timer waits are 2,939
and 0 us. The first receiver also waits 62,656 us for already running preparation.
The changed pipe retry cadence did not remove that preparation cost.

The first source summary triggers a runtime snapshot, but the log records only
`runtime_trace_save_failed` at `2026-10-19T08:40:06.8515827Z`. At that time an
offline `go tool trace` reader still had the previous primary snapshot open.
The original error was not retained, so file sharing is a candidate, not the
confirmed cause of that particular failed save.

A native regression test reproduces an open previous snapshot preventing atomic
replacement; the tested NT rename reports access denied (5). Saving now uses one
fixed spare slot for native access/sharing/lock failures (5/32/33). Error 5 is
not classified as proof of sharing contention. Both files remain protected and complete; an occupied spare
is a real reported failure. No arbitrary file names, ACL relaxation or partial
trace publication is introduced. Retention is at most two 8-MiB snapshots plus
the one temporary file being written, removed on failed publication.

The ordinary fixed-code log distinguishes snapshot size limit, sharing/lock
violation, access denial and unclassified failures. Raw traces remain excluded from normal
support export and peer transfer. Offline analysis should copy the newest
snapshot to a scratch path first, then read that copy rather than either live
slot. The capture still happens at most once per opt-in agent run.

This change repairs diagnostic evidence retention; it is not a field latency
fix. The g344 preparation wait still needs a representative trial and further
causal evidence before changing worker lifetime or admission policy.
