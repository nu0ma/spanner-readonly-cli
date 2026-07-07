package cli

import (
	"strings"
	"testing"
)

func TestTablesStatement(t *testing.T) {
	stmt := tablesStatement()
	if !strings.Contains(stmt.SQL, "information_schema.tables") {
		t.Fatalf("SQL: %s", stmt.SQL)
	}
	if !strings.Contains(stmt.SQL, "table_schema = ''") {
		t.Fatalf("must be restricted to user tables: %s", stmt.SQL)
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
