package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/google/go-cmp/cmp"
)

// TestE2E drives the CLI end-to-end against a local Spanner Omni server.
// It creates a dedicated throwaway database and drops it afterwards.
// Omni provides a fixed default project and instance, both named "default".
//
//	docker run -d --name spanner-omni -p 15000-15026:15000-15026 \
//	  -v spanner:/spanner \
//	  us-docker.pkg.dev/spanner-omni/images/spanner-omni:2026.r1-beta.2 \
//	  start-single-server
//	SPANNER_ENDPOINT=localhost:15000 go test ./...
func TestE2E(t *testing.T) {
	endpoint := os.Getenv("SPANNER_ENDPOINT")
	if endpoint == "" {
		t.Skip("SPANNER_ENDPOINT not set; skipping E2E test against Spanner Omni")
	}

	databaseID := fmt.Sprintf("e2e-%d", time.Now().UnixNano()%1_000_000_000)
	env := envFrom(map[string]string{
		"SPANNER_PROJECT":  "default",
		"SPANNER_INSTANCE": "default",
		"SPANNER_DATABASE": databaseID,
		"SPANNER_ENDPOINT": endpoint,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	setupOmniDatabase(t, ctx, endpoint, databaseID)

	run := func(args ...string) (int, string, string) {
		var stdout, stderr bytes.Buffer
		code := Run(args, &stdout, &stderr, env)
		return code, stdout.String(), stderr.String()
	}
	mustResult := func(t *testing.T, args ...string) Result {
		t.Helper()
		code, stdout, stderr := run(args...)
		if code != 0 {
			t.Fatalf("exit=%d stderr=%s", code, stderr)
		}
		var res Result
		dec := json.NewDecoder(strings.NewReader(stdout))
		dec.UseNumber()
		if err := dec.Decode(&res); err != nil {
			t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout)
		}
		return res
	}

	t.Run("tables", func(t *testing.T) {
		res := mustResult(t, "tables")
		want := Result{
			Columns: []string{"table_schema", "table_name", "parent_table_name"},
			Rows: []map[string]any{
				{"table_schema": "", "table_name": "Users", "parent_table_name": nil},
				{"table_schema": "sales", "table_name": "Users", "parent_table_name": nil},
			},
			RowCount: 2,
		}
		if diff := cmp.Diff(want, res); diff != "" {
			t.Fatalf("tables mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("describe", func(t *testing.T) {
		for _, table := range []string{"Users", "users"} {
			t.Run(table, func(t *testing.T) {
				res := mustResult(t, "describe", table)
				if res.RowCount != 3 {
					t.Fatalf("want 3 columns, got %+v", res)
				}
				if res.Rows[0]["column_name"] != "UserId" || res.Rows[0]["spanner_type"] != "INT64" {
					t.Fatalf("got %+v", res.Rows[0])
				}
			})
		}
	})

	t.Run("describe named schema", func(t *testing.T) {
		for _, table := range []string{"sales.Users", "SALES.users"} {
			t.Run(table, func(t *testing.T) {
				res := mustResult(t, "describe", table)
				names := make([]string, 0, len(res.Rows))
				for _, row := range res.Rows {
					names = append(names, fmt.Sprint(row["column_name"]))
				}
				if diff := cmp.Diff([]string{"UserId", "Name", "Email", "Note"}, names); diff != "" {
					t.Fatalf("column names mismatch (-want +got):\n%s", diff)
				}
			})
		}
	})

	t.Run("indexes", func(t *testing.T) {
		for _, table := range []string{"Users", "users", "sales.Users", "SALES.users"} {
			t.Run(table, func(t *testing.T) {
				res := mustResult(t, "indexes", "--table", table)
				names := make([]string, 0, len(res.Rows))
				for _, row := range res.Rows {
					names = append(names, fmt.Sprint(row["index_name"]))
				}
				if diff := cmp.Diff([]string{"PRIMARY_KEY", "UsersByEmail"}, names); diff != "" {
					t.Fatalf("index names mismatch (-want +got):\n%s", diff)
				}
			})
		}
	})

	t.Run("index definitions", func(t *testing.T) {
		res := mustResult(t, "indexes")
		want := Result{
			Columns: []string{"table_schema", "table_name", "index_name", "index_type", "is_unique", "index_state", "index_columns"},
			Rows: []map[string]any{
				{
					"table_schema": "",
					"table_name":   "Users",
					"index_name":   "PRIMARY_KEY",
					"index_type":   "PRIMARY_KEY",
					"is_unique":    true,
					"index_state":  nil,
					"index_columns": []any{
						map[string]any{"column_name": "UserId", "ordinal_position": json.Number("1"), "column_ordering": "ASC"},
					},
				},
				{
					"table_schema": "",
					"table_name":   "Users",
					"index_name":   "UsersByEmail",
					"index_type":   "INDEX",
					"is_unique":    false,
					"index_state":  "READ_WRITE",
					"index_columns": []any{
						map[string]any{"column_name": "Email", "ordinal_position": json.Number("1"), "column_ordering": "ASC"},
					},
				},
				{
					"table_schema": "sales",
					"table_name":   "Users",
					"index_name":   "PRIMARY_KEY",
					"index_type":   "PRIMARY_KEY",
					"is_unique":    true,
					"index_state":  nil,
					"index_columns": []any{
						map[string]any{"column_name": "UserId", "ordinal_position": json.Number("1"), "column_ordering": "DESC"},
					},
				},
				{
					"table_schema": "sales",
					"table_name":   "Users",
					"index_name":   "UsersByEmail",
					"index_type":   "INDEX",
					"is_unique":    false,
					"index_state":  "READ_WRITE",
					"index_columns": []any{
						map[string]any{"column_name": "Email", "ordinal_position": json.Number("1"), "column_ordering": "DESC"},
						map[string]any{"column_name": "Name", "ordinal_position": json.Number("2"), "column_ordering": "ASC"},
						map[string]any{"column_name": "Note", "ordinal_position": nil, "column_ordering": nil},
					},
				},
			},
			RowCount: 4,
		}
		if diff := cmp.Diff(want, res); diff != "" {
			t.Fatalf("indexes mismatch (-want +got):\n%s", diff)
		}

		filtered := mustResult(t, "indexes", "--table", "SALES.users", "--max-rows", "2")
		wantFiltered := Result{
			Columns:  want.Columns,
			Rows:     want.Rows[2:],
			RowCount: 2,
		}
		if diff := cmp.Diff(wantFiltered, filtered); diff != "" {
			t.Fatalf("filtered indexes mismatch (-want +got):\n%s", diff)
		}

		limited := mustResult(t, "indexes", "--table", "sales.Users", "--max-rows", "1")
		wantLimited := Result{
			Columns:   want.Columns,
			Rows:      want.Rows[2:3],
			RowCount:  1,
			Truncated: true,
		}
		if diff := cmp.Diff(wantLimited, limited); diff != "" {
			t.Fatalf("limited indexes mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("missing schema does not match default schema", func(t *testing.T) {
		for _, req := range [][]string{{"describe", "missing.Users"}, {"indexes", "--table", "missing.Users"}} {
			t.Run(req[0], func(t *testing.T) {
				res := mustResult(t, req...)
				if res.RowCount != 0 || len(res.Rows) != 0 || len(res.Columns) == 0 || res.Truncated {
					t.Fatalf("want empty result with columns, got %+v", res)
				}
			})
		}
	})

	t.Run("query named table", func(t *testing.T) {
		res := mustResult(t, "query", "SELECT Name FROM sales.Users WHERE UserId = 1", "--timeout", "2s")
		want := Result{
			Columns: []string{"Name"},
			Rows: []map[string]any{
				{"Name": "Sales Alice"},
			},
			RowCount: 1,
		}
		if diff := cmp.Diff(want, res); diff != "" {
			t.Fatalf("named table result mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("invalid timeouts are not retryable", func(t *testing.T) {
		for _, timeout := range []string{"0s", "-1s"} {
			t.Run(timeout, func(t *testing.T) {
				code, stdout, stderr := run("query", "SELECT 1 AS id", "--timeout", timeout)
				if code != 2 || stdout != "" {
					t.Fatalf("code=%d stdout=%q stderr=%s", code, stdout, stderr)
				}
				var got errorResult
				if err := json.Unmarshal([]byte(stderr), &got); err != nil {
					t.Fatalf("stderr must be JSON: %v", err)
				}
				if got.Code != "InvalidArgument" || got.Retryable || !strings.Contains(got.Error, "--timeout") {
					t.Fatalf("unexpected error: %+v", got)
				}
			})
		}
	})

	t.Run("query with trailing param flag", func(t *testing.T) {
		res := mustResult(t, "query",
			"SELECT Name, UserId FROM Users WHERE Email = @email",
			"--param", "email=alice@example.com")
		if res.RowCount != 1 || res.Rows[0]["Name"] != "Alice" {
			t.Fatalf("got %+v", res)
		}
	})

	t.Run("INT64 precision survives JSON round-trip", func(t *testing.T) {
		res := mustResult(t, "query", "SELECT UserId FROM Users WHERE Name = 'MaxInt'")
		if got := fmt.Sprint(res.Rows[0]["UserId"]); got != "9223372036854775807" {
			t.Fatalf("got %s", got)
		}
	})

	t.Run("row limits", func(t *testing.T) {
		cases := []struct {
			name          string
			req           string
			wantCount     int
			wantTruncated bool
		}{
			{
				name:          "more rows than limit",
				req:           "1",
				wantCount:     1,
				wantTruncated: true,
			},
			{
				name:      "exactly the limit",
				req:       "2",
				wantCount: 2,
			},
			{
				name:      "fewer rows than limit",
				req:       "3",
				wantCount: 2,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				res := mustResult(t, "query", "SELECT UserId FROM Users ORDER BY UserId", "--max-rows", tc.req)
				if res.RowCount != tc.wantCount || len(res.Rows) != tc.wantCount || res.Truncated != tc.wantTruncated {
					t.Fatalf("got %+v, want %d rows and truncated=%t", res, tc.wantCount, tc.wantTruncated)
				}
				if got := fmt.Sprint(res.Rows[0]["UserId"]); got != "1" {
					t.Fatalf("first row: got UserId %s, want 1", got)
				}
			})
		}
	})

	t.Run("default and unlimited row limits", func(t *testing.T) {
		const sql = "SELECT n FROM UNNEST(GENERATE_ARRAY(1, 101)) AS n ORDER BY n"
		res := mustResult(t, "query", sql)
		if res.RowCount != 100 || len(res.Rows) != 100 || !res.Truncated {
			t.Fatalf("got %d rows, rowCount=%d, truncated=%t; want 100 rows and truncated=true", len(res.Rows), res.RowCount, res.Truncated)
		}
		res = mustResult(t, "query", sql, "--max-rows", "0")
		if res.RowCount != 101 || len(res.Rows) != 101 || res.Truncated {
			t.Fatalf("got %d rows, rowCount=%d, truncated=%t; want 101 rows and truncated=false", len(res.Rows), res.RowCount, res.Truncated)
		}
	})

	t.Run("duplicate columns return a JSON error", func(t *testing.T) {
		cases := []struct {
			name string
			req  string
		}{
			{
				name: "with rows",
				req:  "SELECT 1 AS id, 2 AS id",
			},
			{
				name: "without rows",
				req:  "SELECT UserId AS id, Name AS id FROM Users WHERE UserId = -1",
			},
			{
				name: "unnamed columns without rows",
				req:  "SELECT 1, 2 FROM Users WHERE UserId = -1",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				code, stdout, stderr := run("query", tc.req)
				if code != 1 || stdout != "" {
					t.Fatalf("code=%d stdout=%q stderr=%s", code, stdout, stderr)
				}
				var got errorResult
				if err := json.Unmarshal([]byte(stderr), &got); err != nil {
					t.Fatalf("stderr must be JSON: %v", err)
				}
				if got.Code != "InvalidArgument" || got.Retryable || !strings.Contains(got.Error, "duplicate result column") {
					t.Fatalf("unexpected error: %+v", got)
				}
			})
		}
	})

	t.Run("DML and DDL are rejected", func(t *testing.T) {
		for _, sql := range []string{
			"INSERT INTO Users (UserId, Name) VALUES (99, 'evil')",
			"UPDATE Users SET Name = 'evil' WHERE UserId = 1",
			"DELETE FROM Users WHERE UserId = 1",
			"CREATE TABLE Evil (Id INT64) PRIMARY KEY (Id)",
		} {
			code, _, stderr := run("query", sql)
			if code == 0 {
				t.Fatalf("write statement must fail: %s", sql)
			}
			var got errorResult
			if err := json.Unmarshal([]byte(stderr), &got); err != nil {
				t.Fatalf("stderr must be JSON: %v: %s", err, stderr)
			}
			if got.Code != "InvalidArgument" || got.Retryable {
				t.Fatalf("unexpected Spanner error: %+v", got)
			}
		}
		res := mustResult(t, "query", "SELECT COUNT(*) AS c FROM Users")
		if got := fmt.Sprint(res.Rows[0]["c"]); got != "2" {
			t.Fatalf("data was modified: count=%s", got)
		}
	})

	t.Run("empty query preserves columns", func(t *testing.T) {
		res := mustResult(t, "query", "SELECT Name AS display_name, UserId AS id FROM Users WHERE UserId = -1")
		want := Result{
			Columns: []string{"display_name", "id"},
			Rows:    []map[string]any{},
		}
		if diff := cmp.Diff(want, res); diff != "" {
			t.Fatalf("empty result mismatch (-want +got):\n%s", diff)
		}
	})
}

func setupOmniDatabase(t *testing.T, ctx context.Context, endpoint, databaseID string) {
	t.Helper()
	const instancePath = "projects/default/instances/default"
	databasePath := instancePath + "/databases/" + databaseID

	dbAdmin, err := database.NewDatabaseAdminClient(ctx, omniClientOptions(endpoint)...)
	if err != nil {
		t.Fatalf("database admin client: %v", err)
	}
	t.Cleanup(func() { dbAdmin.Close() })

	op, err := dbAdmin.CreateDatabase(ctx, &databasepb.CreateDatabaseRequest{
		Parent:          instancePath,
		CreateStatement: "CREATE DATABASE `" + databaseID + "`",
		ExtraStatements: []string{
			`CREATE TABLE Users (
				UserId INT64 NOT NULL,
				Name STRING(100),
				Email STRING(200)
			) PRIMARY KEY (UserId)`,
			`CREATE INDEX UsersByEmail ON Users(Email)`,
			`CREATE SCHEMA sales`,
			`CREATE TABLE sales.Users (
				UserId INT64 NOT NULL,
				Name STRING(100),
				Email STRING(200),
				Note STRING(100)
			) PRIMARY KEY (UserId DESC)`,
			`CREATE INDEX sales.UsersByEmail ON sales.Users(Email DESC, Name ASC) STORING (Note)`,
		},
	})
	if err != nil {
		t.Fatalf("create database: %v", err)
	}
	if _, err := op.Wait(ctx); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := dbAdmin.DropDatabase(cleanupCtx, &databasepb.DropDatabaseRequest{Database: databasePath}); err != nil {
			t.Logf("cleanup: drop database: %v", err)
		}
	})

	client, err := spanner.NewClientWithConfig(ctx, databasePath,
		spanner.ClientConfig{IsExperimentalHost: true},
		omniClientOptions(endpoint)...)
	if err != nil {
		t.Fatalf("spanner client: %v", err)
	}
	defer client.Close()
	_, err = client.Apply(ctx, []*spanner.Mutation{
		spanner.Insert("Users",
			[]string{"UserId", "Name", "Email"},
			[]any{int64(1), "Alice", "alice@example.com"}),
		spanner.Insert("Users",
			[]string{"UserId", "Name"},
			[]any{int64(9223372036854775807), "MaxInt"}),
		spanner.Insert("sales.Users",
			[]string{"UserId", "Name", "Email", "Note"},
			[]any{int64(1), "Sales Alice", "sales@example.com", "stored value"}),
	})
	if err != nil {
		t.Fatalf("seed data: %v", err)
	}
}
