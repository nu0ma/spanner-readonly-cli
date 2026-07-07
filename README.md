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
go install github.com/nu0ma/spanner-readonly-cli@latest   # or: go build -o spanner-ro .
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

Tip: with [direnv](https://direnv.net/), drop an `.envrc` in your project
so the variables are set automatically per directory:

```sh
# .envrc
export SPANNER_PROJECT=my-project
export SPANNER_INSTANCE=my-instance
export SPANNER_DATABASE=my-database
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
the data is verified unchanged. CI runs both (unit + E2E) on every push
and pull request.

## Release flow

Releases are managed by [tagpr](https://github.com/Songmu/tagpr):

1. Merge changes into `main` — tagpr opens (or updates) a release PR that
   bumps `version.go` and updates `CHANGELOG.md`
2. Merge the release PR — tagpr tags `vX.Y.Z` and creates a GitHub Release
3. Label the release PR with `minor` / `major` to control the bump
   (default is patch)
