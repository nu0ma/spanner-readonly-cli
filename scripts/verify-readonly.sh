#!/usr/bin/env bash
# Black-box verification of the read-only guarantee using the compiled binary.
#
# Prerequisites:
#   - a Spanner Omni server (docker container name: spanner-omni)
#   - the spanner-ro binary (path via $SPANNER_RO, default ./spanner-ro)
#
# Creates a throwaway sample database, confirms reads work, confirms every
# write statement is rejected by the server, and confirms the data is intact.
set -euo pipefail

SPANNER_RO="${SPANNER_RO:-./spanner-ro}"
DB="readonly-verify-$$"

export SPANNER_ENDPOINT="${SPANNER_ENDPOINT:-localhost:15000}"
export SPANNER_PROJECT=default
export SPANNER_INSTANCE=default
export SPANNER_DATABASE="$DB"

cleanup() {
  docker exec spanner-omni /google/spanner/bin/spanner databases delete "$DB" --quiet || true
}
trap cleanup EXIT

echo "==> creating sample database $DB"
docker exec spanner-omni /google/spanner/bin/spanner databases create-sample-db retail --database-name="$DB"

echo "==> reads must succeed"
"$SPANNER_RO" tables
before=$("$SPANNER_RO" query "SELECT COUNT(*) AS c FROM Users")
echo "before: $before"

echo "==> write statements must be rejected"
while IFS= read -r sql; do
  if out=$("$SPANNER_RO" query "$sql" 2>&1); then
    echo "FAIL: statement was NOT rejected: $sql" >&2
    echo "$out" >&2
    exit 1
  fi
  echo "rejected as expected: $sql"
  echo "  $out"
done <<'EOF'
UPDATE Users SET Email = 'hacked@evil.com' WHERE UserID = 1
DELETE FROM Users WHERE UserID > 0
INSERT INTO Users (UserID, Email) VALUES (999, 'evil@evil.com')
CREATE TABLE Evil (Id INT64) PRIMARY KEY (Id)
DROP TABLE Users
EOF

echo "==> data must be intact"
after=$("$SPANNER_RO" query "SELECT COUNT(*) AS c FROM Users")
echo "after:  $after"
if [ "$before" != "$after" ]; then
  echo "FAIL: data changed" >&2
  exit 1
fi

echo "OK: read-only guarantee verified"
