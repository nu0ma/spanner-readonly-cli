package cli

import (
	"encoding/json"
	"reflect"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/protobuf/types/known/structpb"
)

func scalarType(code sppb.TypeCode) *sppb.Type {
	return &sppb.Type{Code: code}
}

func TestDecodeValueScalars(t *testing.T) {
	cases := []struct {
		name string
		typ  *sppb.Type
		val  *structpb.Value
		want any
	}{
		{"null", scalarType(sppb.TypeCode_INT64), structpb.NewNullValue(), nil},
		{"int64 keeps precision", scalarType(sppb.TypeCode_INT64), structpb.NewStringValue("9223372036854775807"), json.Number("9223372036854775807")},
		{"float64", scalarType(sppb.TypeCode_FLOAT64), structpb.NewNumberValue(1.5), 1.5},
		{"float64 NaN as string", scalarType(sppb.TypeCode_FLOAT64), structpb.NewStringValue("NaN"), "NaN"},
		{"string", scalarType(sppb.TypeCode_STRING), structpb.NewStringValue("abc"), "abc"},
		{"bool", scalarType(sppb.TypeCode_BOOL), structpb.NewBoolValue(true), true},
		{"bytes stays base64", scalarType(sppb.TypeCode_BYTES), structpb.NewStringValue("aGVsbG8="), "aGVsbG8="},
		{"numeric as string", scalarType(sppb.TypeCode_NUMERIC), structpb.NewStringValue("3.14"), "3.14"},
		{"timestamp as string", scalarType(sppb.TypeCode_TIMESTAMP), structpb.NewStringValue("2026-07-07T00:00:00Z"), "2026-07-07T00:00:00Z"},
		{"date as string", scalarType(sppb.TypeCode_DATE), structpb.NewStringValue("2026-07-07"), "2026-07-07"},
		{"json as raw message", scalarType(sppb.TypeCode_JSON), structpb.NewStringValue(`{"a":1}`), json.RawMessage(`{"a":1}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeValue(tc.typ, tc.val)
			if err != nil {
				t.Fatalf("decodeValue error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestDecodeValueArray(t *testing.T) {
	typ := &sppb.Type{Code: sppb.TypeCode_ARRAY, ArrayElementType: scalarType(sppb.TypeCode_INT64)}
	val := structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
		structpb.NewStringValue("1"),
		structpb.NewNullValue(),
		structpb.NewStringValue("2"),
	}})
	got, err := decodeValue(typ, val)
	if err != nil {
		t.Fatalf("decodeValue error: %v", err)
	}
	want := []any{json.Number("1"), nil, json.Number("2")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestDecodeValueStruct(t *testing.T) {
	typ := &sppb.Type{
		Code: sppb.TypeCode_STRUCT,
		StructType: &sppb.StructType{Fields: []*sppb.StructType_Field{
			{Name: "id", Type: scalarType(sppb.TypeCode_INT64)},
			{Name: "name", Type: scalarType(sppb.TypeCode_STRING)},
		}},
	}
	val := structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
		structpb.NewStringValue("7"),
		structpb.NewStringValue("foo"),
	}})
	got, err := decodeValue(typ, val)
	if err != nil {
		t.Fatalf("decodeValue error: %v", err)
	}
	want := map[string]any{"id": json.Number("7"), "name": "foo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
