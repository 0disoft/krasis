# Source write and inbound preparation failures

Paired g327 collection `c9854838a235a576b9e16157af307409` contains three
distinct failed connections. Correlate the shared connection/exchange tags,
not absolute timestamps between PCs: A and B clocks differ. A first records
source heartbeat failure at the write stage, then a heartbeat dispatch failure
on a replacement connection. B later records a failed control request during
A's desktop transitions. Both protected services remain READY.

## Source write evidence

A deterministic test shows the client discarding native errors, write deadline
errors and short writes by replacing them with ErrAuthority. The shared frame
writer had already replaced the original transport error with ErrProtocol.

Source command writes now preserve their original cause alongside ErrAuthority.
Deadline configuration and actual write failures have distinct fixed event codes,
plus a bounded reason and the existing native-code classification. Session-end
records can recover the original numeric Win32 error through the error chain.
Neither raw error text nor input/peer/path contents are logged.

Every failed or partial write still closes the channel, clears the capture epoch
and sends no retry. Zero returned bytes do not prove the worker saw no command.
Request deadlines, authority checks and native retirement proof are unchanged.
Tests reproduce the lost cause before the fix and verify cause preservation,
closed authority, cleared epoch and no replay afterwards.

The g327 snapshot does not contain the original write error. This change cannot
identify its cause retroactively or establish that its trigger is repaired.

No public API, peer wire, database, dependency, ACL or runner change.
