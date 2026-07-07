package main

import "cloud.google.com/go/spanner"

// Metadata queries mirror the tools of spanner-readonly-mcp. table_schema = ''
// restricts results to user tables (GoogleSQL databases).

func tablesStatement() spanner.Statement {
	return spanner.Statement{SQL: `SELECT table_name, parent_table_name
FROM information_schema.tables
WHERE table_schema = ''
ORDER BY table_name`}
}

func describeStatement(table string) spanner.Statement {
	return spanner.Statement{
		SQL: `SELECT column_name, spanner_type, is_nullable, ordinal_position
FROM information_schema.columns
WHERE table_schema = '' AND table_name = @table
ORDER BY ordinal_position`,
		Params: map[string]any{"table": table},
	}
}

func indexesStatement(table string) spanner.Statement {
	stmt := spanner.Statement{SQL: `SELECT table_name, index_name, index_type, is_unique, index_state
FROM information_schema.indexes
WHERE table_schema = ''`}
	if table != "" {
		stmt.SQL += ` AND table_name = @table`
		stmt.Params = map[string]any{"table": table}
	}
	stmt.SQL += `
ORDER BY table_name, index_name`
	return stmt
}
