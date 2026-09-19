package main

import (
	"strings"

	"cloud.google.com/go/spanner"
)

// Metadata queries include default and named user schemas (GoogleSQL databases).

func tablesStatement() spanner.Statement {
	return spanner.Statement{SQL: `SELECT table_schema, table_name, parent_table_name
FROM information_schema.tables
WHERE LOWER(table_schema) NOT IN ('information_schema', 'spanner_sys')
ORDER BY table_schema, table_name`}
}

func describeStatement(table string) spanner.Statement {
	return spanner.Statement{
		SQL: `SELECT column_name, spanner_type, is_nullable, ordinal_position
FROM information_schema.columns
WHERE LOWER(table_schema) = LOWER(@schema) AND LOWER(table_name) = LOWER(@table)
ORDER BY ordinal_position`,
		Params: tableParams(table),
	}
}

func indexesStatement(table string) spanner.Statement {
	stmt := spanner.Statement{SQL: `SELECT i.table_schema, i.table_name, i.index_name, i.index_type, i.is_unique, i.index_state,
  ARRAY(
    SELECT AS STRUCT c.column_name, c.ordinal_position, c.column_ordering
    FROM information_schema.index_columns AS c
    WHERE c.table_catalog = i.table_catalog AND c.table_schema = i.table_schema
      AND c.table_name = i.table_name AND c.index_name = i.index_name
    ORDER BY c.ordinal_position IS NULL, c.ordinal_position, c.column_name
  ) AS index_columns
FROM information_schema.indexes AS i
WHERE LOWER(i.table_schema) NOT IN ('information_schema', 'spanner_sys')`}
	if table != "" {
		stmt.SQL += ` AND LOWER(i.table_schema) = LOWER(@schema) AND LOWER(i.table_name) = LOWER(@table)`
		stmt.Params = tableParams(table)
	}
	stmt.SQL += `
ORDER BY i.table_schema, i.table_name, i.index_name`
	return stmt
}

func tableParams(table string) map[string]any {
	schema, name, qualified := strings.Cut(table, ".")
	if !qualified {
		schema, name = "", table
	}
	return map[string]any{"schema": schema, "table": name}
}
