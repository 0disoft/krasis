# g182 grant exchange correlation

g181's slowest retained A source RequestToGrant span was 120.878ms, while its
grant decoder recorded that interval before first byte progress. Existing
source exchange/progress and receiver request-to-grant spans already cover the
local phases. Their independent attempt ordinals do not establish that an A
span and a B span belong to the same wire request. Another local timer alone
would not remove that evidence gap.

This build adds a bounded private diagnostic tag to the existing handoff profile.
It uses the current authenticated TLS connection and the existing request's
private identities, with a separate exporter label/context, to derive matching
opaque tags on the two peers. It adds no peer protocol field or timing authority.
Only a digest reaches logs/support; raw identities and exporter bytes stay local.
Reconnect, session and epoch changes separate tags. Missing diagnostics never
block or retry input. The cursor mismatch probe and protected input lease rules
remain intact. The schema/clock/privacy contract is in `handoff-profile.md`.

## Validation

Transport tests exercise real local mutual TLS: peer equality, different attempts,
unambiguous identifier boundaries, reconnect and label separation, incomplete
connections and bounded identifier inputs. Collector tests cover unequal local
ordinals, 12-attempt/window limits, invalid or unavailable tags, reentrant
retirement during derivation and a frozen result. Agent tests cover supervisor
forwarding and connection rebinding. Export tests retain tags through local/peer
sanitization while dropping raw identities and rejecting invalid tag shapes.

Desktop tests and vet (`-unsafeptr=false`) passed for agent, transport, peer,
residentagent, handoff, inputworker, productlog, localapi, wininput and application.
Formatting and diff checks passed. A focused transport test/vet slice passed
before the independent transport commit. TLS fixture tests use local sockets;
they do not change real device connections, services or certificates.

Native PIN trials, live paired-device matching and controlled latency comparisons
remain unverified until both laptops run g182. This change provides correlation;
it does not claim to reduce the recorded grant wait or native input delay.

## Paired-device trial

Update both apps and protected input workers to g182. In the fresh collection
window, use A's mouse for six to ten A-to-B entries and return to A after each.
Include the B PIN-open return/reentry case if available. Export both diagnostics
promptly, especially after a slow, blocked or disconnected transition.

Analysis must select this build, join nonempty tags uniquely across one source
and one receiver, and retain missing/duplicate/failed/incomplete cases. Inspect
the same request's A grant wait, A read/handler progress and B request handling,
preparation and grant write as separate host-local spans. No cross-host clock
subtraction, synthetic sum of maxima, pure network residual or speedup claim is
supported by this trial. Later crossings outside the collector cap lack profiles.

Procedures: feature, desktop-app, performance-measurement-integrity-review and
security-privacy-review. Updated source-of-truth: handoff diagnostic profile
contract. No external API, database, runner, peer protocol, installer security,
certificate, service authority or active process changes.
