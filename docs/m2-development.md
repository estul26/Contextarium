# M2 development guide

**IMPLEMENTATION CANDIDATE — FINAL ACCEPTANCE PENDING; D6/R3 OPEN**

Use only synthetic/disposable data on loopback. The owner authorized implementing
D1–D5 from `05907846c59d909990d0c6159edce1da88dea7c6`; this is not authorization
to merge, deploy, use personal data, begin M3, or set up R3 fault infrastructure.
See the [contract](m2-contract.md), [test plan](m2-test-plan.md), and
[evidence matrix](m2-evidence.md). Earlier M0/M1 guides describe their historical
binaries; their PATCH examples without a base are not new M2 requests.

## API use

Create subjects, register exact schema versions, and create a record using the
M1 request fields. A newly created M2 record includes `revision: 1`. Every record
mutation needs an `Idempotency-Key`. For PATCH, send the observed revision as an
ordinary positive decimal JSON integer alongside replacement fields:

```json
{"base_revision":1,"data":{"title":"Synthetic task","done":true},"status":"archived"}
```

The five replacement fields remain `data`, `key`, `sensitivity`, `provenance`, and
`status`. Omitted fields persist. Supplied data/provenance replace the whole field;
null clears key/provenance only. A valid nonempty no-op creates a new revision.
A missing base is 400 `BASE_REVISION_REQUIRED`; a stale base is 409
`REVISION_CONFLICT` with expected/current revision numbers. Re-read and deliberately
rebase using a new key. `If-Match` is not supported as an alternate precondition.

History routes are:

- `GET /api/v1/records/{id}/revisions?limit=20&cursor=...`
- `GET /api/v1/records/{id}/revisions/{revision}`
- `POST /api/v1/records/{id}/restore/{revision}` with `{"base_revision":3}`

Lists default to 20, accept 1–100, and use an opaque record-bound cursor. A traversal
captures its original head, so later appends do not move its end. Restore appends
historical content using new attribution/time; it is different from status-only
unarchiving. There is no audit query API or history editing/deletion API.

The local HTTP component is explicitly attributed as `development_test` /
`local-http-adapter`, with internally generated request IDs. This does not identify
an authenticated person. Arbitrary actor/client/auth/request headers do not supply
trusted attribution. Internal service calls must use `engine.WithAttribution` with
an explicit development/test actor and a generated `req_` identifier.

## Atomic mutation and replay

The existing IMMEDIATE transaction first checks the scoped idempotency key and
canonical request hash. A matching committed result is returned before checking
its old base against the current head. Different input conflicts. For new requests:
validate shape/base, read/check head and exact schema, append snapshot, conditionally
write current state, append required event, check bounded linkage, encode/store the
result and contract tag, and commit. Any earlier error rolls the entire transaction
back. A commit or response-delivery failure may be ambiguous: retry the same key
and original request. Do not switch keys merely because a response was lost.

## Migration and rollback

Migration 003 is appended; migrations 001 and 002 are unchanged. It adds revisions,
minimal audit, current-head guards and the replay contract discriminator in the
same transaction as adoption and the migration ledger. Every existing M1 record,
including archived records, is adopted as revision 1. Its stored content and
created/updated times remain unchanged. The new revision has `m1_adoption`, the
actual adoption timestamp, actor `local-migration-003`, and an event marked
`m1_history_boundary`. Nothing reconstructs earlier revisions from timestamps or
old replay responses. Fresh/empty databases have no adoption events.

Existing idempotency rows retain their scope, key, hash and exact response text.
Matching M1 record requests replay their original object without a revision,
including after later M2 edits/restart; response metadata says `result_contract:m1`.
New M2 results use `m2`. Adding a base to an old keyed request changes its hash and
conflicts. A new PATCH key without a base fails. Subject/schema replay remains intact.

Do not downgrade a schema-003 file or remove ledger/history rows. Old M0/M1 binaries
refuse it before listening. To return to M1, close all processes/connections and use
a separately preserved closed M1 copy; preserve the M2 file independently and
explicitly account for later M2 changes omitted by that rollback. Never copy only
a live main database while its WAL contains state.

### Reviewed-candidate 003 checksum change

The PR #6 correction aligns restore audit events with the approved
`revision.restored`. Because 003 is still unmerged, its constraint and trigger were
corrected in place; 001/002 are unchanged. The reviewed implementation
`3da25a78e5100d58fb75b196bc91a85e606f0b78` used 003 checksum
`31865ddf5b43c6d64b741d8be40c46da191fd01d77681b88c4aecf9ebbae01cf`;
the corrected checksum is
`9e6bfd1718cdaeaed92b7ac028bb90894d4a1ca3467d7ca02e6ba8460dbb5b9d`.

A disposable database created/adopted by that reviewed candidate is incompatible
with this binary and will be refused before listening. Preserve it unchanged; do
not edit its migration ledger, overwrite it, or assume it upgrades automatically.
Use a fresh disposable database or a separately prepared closed M1 fixture.
Retain old candidate evidence separately. This correction provides no old-003
conversion and does not change M1 adoption or legacy replay policy.

## Validation

```sh
gofmt -l .
git diff --check
go mod verify
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./...
go build -o ./bin/contextarium ./cmd/contextarium
```

The Go suites own temporary databases and child processes. Internal barriers are
unexported test instrumentation, with no production environment switch or HTTP
control. The binary rehearsal requires separately built M0/M1 checkpoint binaries
and a candidate binary; all database paths are created by the script:

```sh
python3 scripts/m2-binary-check.py \
  --binary ./bin/contextarium \
  --m1-binary ./test-artifacts/m1-contextarium \
  --m0-binary ./test-artifacts/m0-contextarium \
  --reviewed-m2-binary ./test-artifacts/reviewed-m2-contextarium
```

The optional `--reviewed-m2-binary` must be built from
`3da25a78e5100d58fb75b196bc91a85e606f0b78`. It creates a separate disposable
old-003 fixture and checks that the corrected candidate refuses it without
rewriting the ledger or mutation stores. It never uses an existing candidate DB.

R1 graceful SIGTERM/drain, R2 abrupt test-child termination, and R3 simulated
storage/power loss are distinct. Passing these commands establishes no R3 result.
The owner subsequently authorized the bounded standard-runner R3 harness described
in the [evidence document](m2-evidence.md). It is excluded from ordinary builds;
`-tags r3` compiles the separate test worker against the pinned SQLite header.
The gated R3 workflow permits only two explicitly marked pushes, rejects reruns,
serializes jobs, and enforces a 60-minute timeout. It does not change ordinary CI.
No artifact/cache upload, paid runner, personal VM or host storage fault is allowed.
Harness validation and negative controls must pass before the application matrix.
Current R3 status remains unexecuted; compilation is not acceptance evidence.
