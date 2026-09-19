package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
)

func TestRowToMap(t *testing.T) {
	row, err := spanner.NewRow([]string{"id", "name", "data"}, []any{int64(5), "x", []byte("hi")})
	if err != nil {
		t.Fatalf("NewRow error: %v", err)
	}
	cols, m, err := rowToMap(row)
	if err != nil {
		t.Fatalf("rowToMap error: %v", err)
	}
	if !reflect.DeepEqual(cols, []string{"id", "name", "data"}) {
		t.Fatalf("columns: got %#v", cols)
	}
	want := map[string]any{"id": json.Number("5"), "name": "x", "data": "aGk="}
	if !reflect.DeepEqual(m, want) {
		t.Fatalf("got %#v, want %#v", m, want)
	}
}

func TestMarshalResult(t *testing.T) {
	res := Result{
		Columns:  []string{"id", "note"},
		Rows:     []map[string]any{{"id": json.Number("9223372036854775807"), "note": "a<b"}},
		RowCount: 1,
	}
	got, err := marshalResult(res)
	if err != nil {
		t.Fatalf("marshalResult error: %v", err)
	}
	s := string(got)
	if !strings.Contains(s, `"id":9223372036854775807`) {
		t.Fatalf("INT64 should stay a JSON number without precision loss: %s", s)
	}
	if !strings.Contains(s, `"note":"a<b"`) {
		t.Fatalf("output must not be HTML-escaped: %s", s)
	}
	if !strings.Contains(s, `"rowCount":1`) {
		t.Fatalf("missing rowCount: %s", s)
	}
	if !strings.Contains(s, `"truncated":false`) {
		t.Fatalf("complete results must explicitly include truncated=false: %s", s)
	}
}

func TestMarshalResultEmptyRows(t *testing.T) {
	got, err := marshalResult(Result{Columns: []string{"id"}, Rows: nil, RowCount: 0})
	if err != nil {
		t.Fatalf("marshalResult error: %v", err)
	}
	if !strings.Contains(string(got), `"rows":[]`) {
		t.Fatalf("empty rows should encode as [] not null: %s", got)
	}
}

func TestRowToMapRejectsDuplicateColumns(t *testing.T) {
	for _, names := range [][]string{{"id", "id"}, {"", ""}} {
		t.Run(strings.Join(names, ","), func(t *testing.T) {
			row, err := spanner.NewRow(names, []any{int64(1), int64(2)})
			if err != nil {
				t.Fatalf("NewRow error: %v", err)
			}
			_, _, err = rowToMap(row)
			if err == nil {
				t.Fatal("want error instead of silently overwriting a column")
			}
			if got := spanner.ErrCode(err); got != codes.InvalidArgument {
				t.Fatalf("error code: got %s, want InvalidArgument", got)
			}
			if !strings.Contains(err.Error(), "use AS") {
				t.Fatalf("error should explain how to fix the query: %v", err)
			}
		})
	}
}
