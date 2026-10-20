# g378: tolerate a confirmed process exit during stage ownership lookup

Windows stage cleanup enumerates product processes before reading their image
paths. A process can exit between the Toolhelp snapshot and OpenProcess, which
returns ERROR_INVALID_PARAMETER and previously aborted the entire cleanup.

On that specific error, cleanup now obtains another complete process snapshot.
Only an absent PID is skipped. A present or reused PID, access denial, snapshot
failure, empty path or relative path still blocks cleanup. No manual stage
deletion or weakening of protected-directory ownership is involved.

The launchrecord suite passes, including a real Windows snapshot/current-process
lookup and controlled exit, PID reuse, access denial and snapshot failure cases.
The first peerupdate suite run failed its unrelated one-second legacy clipboard
notification deadline; both the isolated clipboard regression and a fresh full
peerupdate suite passed on rerun. Paired deployment evidence follows after
applying the committed build.

No product API, database, dependency, runner, firewall or signing change. No push
or issue comment. This fixes a startup cleanup race; other cleanup failures must
still be reported rather than classified as success.
