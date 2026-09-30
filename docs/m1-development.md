# M1 synthetic development

M1 is local-only and uses disposable data. The source of truth for endpoint
semantics is [m1-contract.md](m1-contract.md). Do not use personal records or
remotely expose this development server. M2 has not started.

## Build and checks

Go 1.26.x and a C compiler are required for the bundled SQLite driver.

```sh
export CGO_ENABLED=1
go mod download
test -z "$(gofmt -l .)"
go vet ./...
go test ./...
go test -race ./...
make build
```

Linux CI runs the same five checks on PRs and pushes to main. The M1 tests cover
canonical namespaces, exact numeric validation, pinned immutable schemas, two
independent synthetic domains, bounded pagination, rejected/atomic patches,
concurrent retries, replay after restart, HTTP limits, browser boundaries, and
M0-to-M1 migration with old-binary refusal. They do not establish personal-data
readiness, authentication, historical recovery, or production durability.

## Local walkthrough

From the repository root, start a fresh synthetic database:

```sh
m1_demo_dir=$(mktemp -d)
CONTEXTARIUM_DB_PATH="$m1_demo_dir/synthetic.db" ./bin/contextarium
```

In another terminal, use `curl` and `jq`:

```sh
base=http://127.0.0.1:8080/api/v1
subject_id=$(curl --fail-with-body -sS "$base/subjects" \
  -H 'Content-Type: application/json' -H 'Idempotency-Key: demo-subject' \
  -d '{"kind":"project","display_name":"Synthetic project"}' | jq -r '.data.id')

curl --fail-with-body -sS "$base/schemas" \
  -H 'Content-Type: application/json' -H 'Idempotency-Key: demo-schema' \
  -d '{"schema_id":"example.task","schema_version":1,"migration_policy":"explicit","definition":{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"title":{"type":"string"},"done":{"type":"boolean"}},"required":["title","done"],"additionalProperties":false}}' | jq

record_id=$(jq -nc --arg subject "$subject_id" \
  '{subject_id:$subject,namespace:"tasks.example",schema_id:"example.task",schema_version:1,data:{title:"Synthetic task",done:false}}' \
  | curl --fail-with-body -sS "$base/records" \
      -H 'Content-Type: application/json' -H 'Idempotency-Key: demo-record' \
      --data-binary @- | jq -r '.data.id')

curl --fail-with-body -sS "$base/records/$record_id" \
  -X PATCH -H 'Content-Type: application/json' -H 'Idempotency-Key: demo-update' \
  -d '{"data":{"title":"Synthetic task","done":true}}' | jq

curl --fail-with-body -sS "$base/records?namespace=tasks.example&limit=20" | jq
```

Expected: one subject, one schema, and one record with `done:true`. Repeating the
same mutation key/body returns the original object without repeating the effect.
Changing a body requires a new key. A different body under the same key returns
409; wrong schema data returns 422; changing namespace/subject/schema pin returns
400. Stop the server with Ctrl-C. Retain or discard only this disposable test
folder after shutdown; no production rollback is claimed.

For rollback to M0, use a fresh path or the closed M0 database copy preserved
before upgrading. Never remove migration-ledger rows to force a downgrade.
