# ADR 0050: Recovery evidence and native owner retention

Recovery evidence distinguishes unknown from confirmed forwarding shutdown,
worker join, and empty pressed ledger. Cancellation stops source forwarding first.
If workers do not join within four seconds, the supervisor reports unverified
recovery and the resident manager latches FAULTED, including while disconnecting.
New work remains rejected even if an old worker finishes later.

The session deliberately retains unfinished workers and the enclosing crash guard
instead of returning early and risking a late native key-down after final release.
The timeout bounds the health report, not the OS call. A permanently hung native
owner requires process exit; in-process code cannot safely cancel an arbitrary
Win32 call. Real OS hang/secure-desktop recovery is not claimed by mocked tests.

Cross-thread AttachThreadInput fallback is disabled in the product implementation.
Ordinary foreground activation is still attempted and verified. If Windows denies
it, crossing fails closed rather than joining another application's input queue.
Old fallback classification tests retain mocked thread-attachment behavior only.

Capture cancellation also retires the run before posting its stop message, so a
late native activation cannot acknowledge a new active owner. Agent generation 18
contains the stability changes documented in ADRs 0046 through 0050.
