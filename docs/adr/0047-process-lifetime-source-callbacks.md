# ADR 0047: Process-lifetime source callbacks

The product source keyboard hook, mouse hook, and wheel-capture window use three
process-lifetime syscall callback trampolines. Reconnects allocate only ordinary
Go closures, registered by the owning OS thread or HWND and removed at teardown.
No Go pointer is passed through a uintptr registry token.

Missing owners forward to the next hook or DefWindowProc. Hook registry removal
follows unhook attempts; window registry removal follows native window cleanup.
Cleanup failures continue to propagate into the existing FAULTED/no-reconnect
policy. This change does not prove recovery from a hung native call.

The churn test exercises 10,000 owner lifetimes without native input injection.
Lab-only capture experiments are not converted by this product-path change.
