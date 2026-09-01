# Local write inventory and secure mutation boundary

- Status: Active
- Owner: resident-agent and local-storage maintainers
- Scope: runtime files created or replaced by Krasis

## Windows mutation rule

Sensitive Windows mutation uses `internal/securefs`. The package opens the target directory with
`OBJ_DONT_REPARSE`, keeps that directory handle for the complete operation, and creates, opens,
renames, or removes leaf objects relative to the handle. A later pathname replacement therefore
does not redirect the mutation. Rename sources must be regular, non-reparse files with exactly one
hard link. No-replace operations reject a pre-existing destination.

Credential and trust files additionally apply and validate their current-user plus `SYSTEM` DACL
through the same file handle. Directory DACLs are applied and validated through the opened
directory handle before secret bytes are generated. Incomplete credential cleanup removes the
known files and directories through their handles; it does not call path-recursive deletion after
a failure.

The non-Windows implementation is a deterministic test boundary using exclusive create, `Lstat`,
regular-file checks, and OS rename. It is not a supported product security claim.

## Writer inventory

| Owner | Data | Sensitivity | Mutation contract |
| --- | --- | --- | --- |
| `internal/evidence` | aggregate JSON reports and guard reports | privacy and integrity | handle-relative temporary file, flush, exclusive no-replace promotion |
| `internal/launcher.SaveConfig` | launcher configuration | integrity | handle-relative temporary file, flush, atomic replace |
| `internal/peercredentials` | CA, device certificates, private keys, peer certificates, manifest | secret and integrity | exclusive protected root and child directories, handle-protected exclusive files, handle cleanup on failure |
| `internal/truststore` | lock file and `trust.json` | authorization and integrity | one directory handle spans lock, bounded read, temporary write, atomic replace, and final DACL validation |
| `internal/launcher.Manager` and `internal/labcli` | bounded stop marker | control coordination | producer and consumer retain a report-directory handle and use exclusive create, validated open, and handle-relative removal |

Build outputs, CI artifacts, downloaded probe kits, and installer outputs are not runtime local-data
writers. Their provenance and signing boundary belongs to `docs/ops/release.md` and issue #16.

## Failure behavior

- Reparse traversal, a non-regular leaf, multiple hard links, destination precreation, invalid leaf
  names, ACL drift, and parent-handle acquisition failure stop the operation.
- Errors classify the failed boundary but public reports do not include absolute paths, credential
  bodies, session identifiers, or input content.
- A failed atomic replacement leaves the previous accepted file intact. Temporary cleanup is best
  effort through the retained handle.
- Credential generation never overwrites an existing bundle and never recursively deletes a path
  that may have been replaced.

## Residual boundary

This boundary prevents pathname substitution from redirecting Krasis mutations. It does not claim
to isolate secrets from arbitrary code already executing as the same Windows user with process
injection, debugging, or direct access to that user's protected files. Signed installation,
executable integrity, and compromise response remain separate requirements.

