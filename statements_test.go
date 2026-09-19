package main

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestTablesStatement(t *testing.T) {
	stmt := tablesStatement()
	if !strings.Contains(stmt.SQL, "information_schema.tables") {
		t.Fatalf("SQL: %s", stmt.SQL)
	}
	if !strings.Contains(stmt.SQL, "NOT IN ('information_schema', 'spanner_sys')") {
		t.Fatalf("must exclude system schemas: %s", stmt.SQL)
	}
}

func TestDescribeStatement(t *testing.T) {
	stmt := describeStatement("Users")
	if !strings.Contains(stmt.SQL, "information_schema.columns") {
		t.Fatalf("SQL: %s", stmt.SQL)
	}
	if !strings.Contains(stmt.SQL, "@table") {
		t.Fatalf("table name must be a bound parameter, not interpolated: %s", stmt.SQL)
	}
	if stmt.Params["table"] != "Users" {
		t.Fatalf("params: %#v", stmt.Params)
	}
}

func TestIndexesStatement(t *testing.T) {
	all := indexesStatement("")
	if !strings.Contains(all.SQL, "information_schema.indexes") {
		t.Fatalf("SQL: %s", all.SQL)
	}
	if strings.Contains(all.SQL, "@table") {
		t.Fatalf("no table filter expected: %s", all.SQL)
	}
	filtered := indexesStatement("Users")
	if !strings.Contains(filtered.SQL, "@table") || filtered.Params["table"] != "Users" {
		t.Fatalf("table filter expected: %s %#v", filtered.SQL, filtered.Params)
	}
}

func TestTableParams(t *testing.T) {
	cases := []struct {
		name string
		req  string
		want map[string]any
	}{
		{
			name: "default schema",
			req:  "Users",
			want: map[string]any{"schema": "", "table": "Users"},
		},
		{
			name: "named schema",
			req:  "Sales.Users",
			want: map[string]any{"schema": "Sales", "table": "Users"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(tc.want, tableParams(tc.req)); diff != "" {
				t.Fatalf("table parameters mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
