# spanner-readonly-cli

Read-only CLI for Cloud Spanner — the CLI counterpart of
[nu0ma/spanner-readonly-mcp](https://github.com/nu0ma/spanner-readonly-mcp).

Every query runs inside a **read-only snapshot transaction**
(`client.Single()`, a single-use `spanner.ReadOnlyTransaction`). The
transaction type exposes no write methods, so writes are impossible by
construction — no SQL filtering or regex blocklists involved. DML/DDL
statements are rejected by the Spanner server itself:

```
$ spanner-readonly-cli query "DELETE FROM Users WHERE UserId=1"
{"error":"spanner: code = \"InvalidArgument\", desc = \"DML statements may not be performed in single-use transactions, to avoid replay.\", ...}
```

## Install

```sh
go install github.com/nu0ma/spanner-readonly-cli@latest
```

The executable is named `spanner-readonly-cli`. Ensure Go's install directory
(`GOBIN`, or `$(go env GOPATH)/bin` by default) is on your `PATH`.

Alternatively, build from the repository root and run the local executable:

```sh
go build -o spanner-readonly-cli .
./spanner-readonly-cli --help
```

## Usage

```sh
export SPANNER_PROJECT=my-project
export SPANNER_INSTANCE=my-instance
export SPANNER_DATABASE=my-database

spanner-readonly-cli tables                  # list user tables
spanner-readonly-cli describe Users          # column definitions
spanner-readonly-cli indexes --table Users   # indexes (filter optional)
spanner-readonly-cli query "SELECT * FROM Users LIMIT 10"
spanner-readonly-cli query "SELECT * FROM Users WHERE Email = @email" --param email=a@example.com
```

For a local build, use `./spanner-readonly-cli` in the examples above.

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
{"columns":["UserId","Name"],"rows":[{"UserId":1,"Name":"Alice"}],"rowCount":1,"truncated":false}
```

Column names are preserved when a query returns no rows:

```json
{"columns":["UserId","Name"],"rows":[],"rowCount":0,"truncated":false}
```

- `INT64` stays a JSON number with full precision (no 2^53 truncation)
- `BYTES` → base64, `NUMERIC`/`TIMESTAMP`/`DATE` → strings, `JSON` → inline JSON
- `ARRAY` → array, `STRUCT` → object, `NULL` → null
- `FLOAT64` NaN/Infinity → strings (JSON has no representation for them)

Duplicate result column names (including repeated unnamed columns) and
duplicate STRUCT field names are rejected instead of silently overwriting values.
Assign unique aliases with `AS` when selecting columns with the same name.

### Result limits

All data commands return at most **100 rows** by default. Override with
`--max-rows N`, or use `--max-rows 0` to return all rows:

```sh
spanner-readonly-cli query "SELECT UserId FROM Users ORDER BY UserId" --max-rows 20
```

`rowCount` counts returned rows. `truncated` is `true` only when an additional
row was found beyond the limit; it is `false` for complete results, including
results with exactly the requested number of rows. Truncated results exit
successfully, so agents should check this field before treating the output as
complete. No continuation token is returned.

The limit bounds the number of returned rows, not their byte size or the
database's query cost. Use selective SQL and an appropriate `LIMIT` when needed.

### Errors

Errors go to stderr as a single JSON object:

```json
{"error":"...","code":"PermissionDenied","retryable":false}
```

The `error` field remains a string, but message text may change; use `code` for
programmatic handling. `code` uses Spanner/gRPC status names;
local argument errors use `InvalidArgument`, missing configuration uses
`FailedPrecondition`, and unclassified local failures use `Unknown`.
`retryable` is `true` for `Unavailable`, `Aborted`, and `DeadlineExceeded`;
retry with backoff and a bounded retry count, adjusting the timeout if needed.

Exit codes are `0` for success (including help), `2` for invalid CLI usage, and
`1` for configuration or execution failures. `--help` and `--version` remain
plain text on stdout.

### Query parameters

`--param name=value` binds a STRING-typed parameter (repeatable). For other
types, cast in SQL:

```sh
spanner-readonly-cli query "SELECT * FROM Users WHERE UserId = CAST(@id AS INT64)" --param id=42
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
2. Label the release PR with `tagpr:minor` / `tagpr:major` to control the bump
   (default is patch)
3. Merge the release PR — tagpr tags `vX.Y.Z` and creates a GitHub Release

For regular change PRs, use `minor` / `major` before merging; tagpr uses those
labels to add the corresponding `tagpr:minor` / `tagpr:major` label to the release PR.

## License

This project is licensed under the [MIT License](LICENSE).
