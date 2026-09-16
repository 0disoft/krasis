# ADR 0048: Terminal fencing for capture commands

The capture deadline includes command serialization, queue admission, and result
delivery. A timeout permanently retires that capture run. Queued commands check
the terminal state before native work; activation checks it between native steps
and commits its active flag under the same lock that publishes terminal failure.
An in-flight native call cannot therefore acknowledge a new active run after
failure. Native calls themselves are not cancellable and may finish late.

Source forwarding stops before capture deactivation. Late wheel messages for the
retired source are discarded, not interpreted as a new source fault. Native
resource cleanup still runs on its owner thread and errors remain observable.

This is not proof of bounded native cleanup: cross-thread foreground fallback and
structured recovery reporting remain separate work. Never infer LOCAL recovery
solely from the command deadline.
