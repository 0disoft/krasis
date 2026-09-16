# Peer-assisted development updates

Status: in progress

## Scope and authorization

The operator requested one-click transfer and application of a development build
to a paired laptop after one local opt-in. Input-control permission alone does
not grant update permission. Opt-in must bind the exact paired certificate, be
revocable locally, and be rechecked before receiving and before applying a bundle.
The existing unsigned development ZIP is not an official signed release. The
opt-in delegates code supply to that paired computer; hashes only detect damage.
No generic remote shell, arbitrary path write, or arbitrary file execution API
is part of this feature.

## Delivery stages

1. Validate and snapshot the complete distribution.
2. Transfer over a separately bounded, mutually authenticated connection so file
   traffic does not hold the input-session write lock.
3. Verify fixed file names, total and per-file size, SHA-256 checksums, platform,
   clean Go build identity, matching revision, and a higher agent generation.
4. Stage in a new user-private directory; never replace the running executables.
5. Recheck permission, drain the input connection, launch the new GUI, and verify
   the expected agent build before reconnecting. Downloaded is not installed.

The first supporting version must be installed manually on both PCs. Unknown or
old peers must not receive a new input protocol message. Retained old directories
provide a manual recovery path; automatic downgrade is not authorized.

## Increment 1

`internal/updatebundle` implements bounded ZIP validation, distribution packing,
and fresh-directory staging. It never executes received code. Transfer,
permissions, GUI application, and installed two-PC verification remain separate
increments. No production signed feed, global rollout, or release publication is
implied by this development workflow.
