# Distinguish installation directory moves (#16)

g398 and A's first g404 install both reported replace/file_move, Win32 5,
with the previous installation restored. That operation covered three directory
renames and journal publication, preventing an exact failure-stage diagnosis.
Directory retirement, activation and restoration now report separate fixed
operation codes. Journal publication retains file_move. Transaction order,
authorization, rollback and sharing policy are unchanged; no retries were added.

A Windows filesystem test reproduces ERROR_ACCESS_DENIED when a protected
child image is open while its parent directory is renamed. Both endpoints stay
unchanged; closing the child handle allows the move. The test verifies all three
operation codes survive receipt validation without exposing paths or native
error text. Existing journal/root-pin and image-mutation tests also pass.
This demonstrates a mechanism, not the identity of the handle owner in either
real installation failure. #16 remains open pending that evidence and signing.

Verification: full internal/inputworker tests, installationdiag compilation,
inputworker/installationdiag vet and git diff --check passed. No physical
installation was repeated solely to provoke the failure. No push, issue comment,
remote ref, dependency, DB, runner or firewall change.

At 00:43Z g404 was installed on both devices, but A remained CONNECTING and B's
fresh support collection 2c896583adfac40e9a08a0edeccbdfe6 recorded LOCAL with
READY service/input health. Paired connection resumption is still unverified.
The user's earlier all-normal report applies to the explicitly requested g377
round trips, clicks, keyboard, drag and vertical/horizontal scrolling; it is not
a current g404 report or completion of the remaining hardware matrix.
