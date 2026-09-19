package main

import (
	"bytes"
	"encoding/json"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Result struct {
	Columns   []string         `json:"columns"`
	Rows      []map[string]any `json:"rows"`
	RowCount  int              `json:"rowCount"`
	Truncated bool             `json:"truncated"`
}

func rowToMap(row *spanner.Row) ([]string, map[string]any, error) {
	columns := row.ColumnNames()
	if err := validateColumnNames(columns); err != nil {
		return nil, nil, err
	}
	m := make(map[string]any, len(columns))
	for i, name := range columns {
		var gcv spanner.GenericColumnValue
		if err := row.Column(i, &gcv); err != nil {
			return nil, nil, err
		}
		decoded, err := decodeValue(gcv.Type, gcv.Value)
		if err != nil {
			return nil, nil, err
		}
		m[name] = decoded
	}
	return columns, m, nil
}

func validateColumnNames(columns []string) error {
	seen := make(map[string]struct{}, len(columns))
	for _, name := range columns {
		if _, exists := seen[name]; exists {
			return status.Errorf(codes.InvalidArgument, "duplicate result column %q; use AS to assign unique column names", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

// marshalResult encodes without HTML escaping so SQL-ish strings (<, >, &)
// come out readable.
func marshalResult(res Result) ([]byte, error) {
	if res.Rows == nil {
		res.Rows = []map[string]any{}
	}
	return marshalJSON(res)
}

func marshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
