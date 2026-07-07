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
		if res.RowCount != 1 || res.Rows[0]["table_name"] != "Users" {
			t.Fatalf("got %+v", res)
		}
	})

	t.Run("describe", func(t *testing.T) {
		res := mustResult(t, "describe", "Users")
		if res.RowCount != 3 {
			t.Fatalf("want 3 columns, got %+v", res)
		}
		if res.Rows[0]["column_name"] != "UserId" || res.Rows[0]["spanner_type"] != "INT64" {
			t.Fatalf("got %+v", res.Rows[0])
		}
	})

	t.Run("indexes", func(t *testing.T) {
		res := mustResult(t, "indexes", "--table", "Users")
		names := make([]string, 0, len(res.Rows))
		for _, row := range res.Rows {
			names = append(names, fmt.Sprint(row["index_name"]))
		}
		if !strings.Contains(strings.Join(names, ","), "UsersByEmail") {
			t.Fatalf("UsersByEmail not found in %v", names)
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
			if !strings.Contains(stderr, `{"error":`) {
				t.Fatalf("stderr should be a JSON error: %s", stderr)
			}
		}
		res := mustResult(t, "query", "SELECT COUNT(*) AS c FROM Users")
		if got := fmt.Sprint(res.Rows[0]["c"]); got != "2" {
			t.Fatalf("data was modified: count=%s", got)
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
	})
	if err != nil {
		t.Fatalf("seed data: %v", err)
	}
}
