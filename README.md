# spanner-ro

Read-only CLI for Cloud Spanner — the CLI counterpart of
[nu0ma/spanner-readonly-mcp](https://github.com/nu0ma/spanner-readonly-mcp).

Every query runs inside a **read-only snapshot transaction**
(`client.Single()`, a single-use `spanner.ReadOnlyTransaction`). The
transaction type exposes no write methods, so writes are impossible by
construction — no SQL filtering or regex blocklists involved. DML/DDL
statements are rejected by the Spanner server itself:

```
$ spanner-ro query "DELETE FROM Users WHERE UserId=1"
{"error":"spanner: code = \"InvalidArgument\", desc = \"DML statements may not be performed in single-use transactions, to avoid replay.\", ...}
```

## Install

```sh
go install spanner-readonly-cli@latest   # or: go build -o spanner-ro .
```

## Usage

```sh
export SPANNER_PROJECT=my-project
export SPANNER_INSTANCE=my-instance
export SPANNER_DATABASE=my-database

spanner-ro tables                  # list user tables
spanner-ro describe Users          # column definitions
spanner-ro indexes --table Users   # indexes (filter optional)
spanner-ro query "SELECT * FROM Users LIMIT 10"
spanner-ro query "SELECT * FROM Users WHERE Email = @email" --param email=a@example.com
```

Flags `--project` / `--instance` / `--database` override the environment
variables. `SPANNER_EMULATOR_HOST` is honored for local development.
Authentication uses Application Default Credentials.

### Spanner Omni

`--endpoint` (or `SPANNER_ENDPOINT`) targets a [Spanner Omni](https://cloud.google.com/products/spanner/omni)
deployment instead of Google Cloud. The connection is unauthenticated
plaintext gRPC (the current Omni preview does not support TLS). Project and
instance are both `default` on Omni:

```sh
spanner-ro tables --endpoint localhost:15000 \
  --project default --instance default --database my-db
```

### Output

A single JSON object on stdout — designed to be easy for agents and `jq`:

```json
{"columns":["UserId","Name"],"rows":[{"UserId":1,"Name":"Alice"}],"rowCount":1}
```

- `INT64` stays a JSON number with full precision (no 2^53 truncation)
- `BYTES` → base64, `NUMERIC`/`TIMESTAMP`/`DATE` → strings, `JSON` → inline JSON
- `ARRAY` → array, `STRUCT` → object, `NULL` → null
- `FLOAT64` NaN/Infinity → strings (JSON has no representation for them)

Errors go to stderr as `{"error":"..."}` with a non-zero exit code.

### Query parameters

`--param name=value` binds a STRING-typed parameter (repeatable). For other
types, cast in SQL:

```sh
spanner-ro query "SELECT * FROM Users WHERE UserId = CAST(@id AS INT64)" --param id=42
```

### Timeout

Queries time out after 30s by default; override with `--timeout 2m`.

## Development

```sh
go test ./...
```

The E2E test (`TestE2E`) runs against a local Spanner Omni server and is
skipped unless `SPANNER_ENDPOINT` is set. It creates a throwaway database
on the fixed `default` project/instance and drops it afterwards:

```sh
docker run -d --name spanner-omni -p 15000-15026:15000-15026 \
  -v spanner:/spanner \
  us-docker.pkg.dev/spanner-omni/images/spanner-omni:2026.r1-beta.2 \
  start-single-server

SPANNER_ENDPOINT=localhost:15000 go test ./...
```

The E2E test covers all four commands plus the read-only guarantee:
INSERT / UPDATE / DELETE / CREATE TABLE are all rejected by the server and
the data is verified unchanged.
