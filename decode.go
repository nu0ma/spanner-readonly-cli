package main

import (
	"encoding/json"
	"fmt"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/protobuf/types/known/structpb"
)

// decodeValue converts a Spanner protobuf value into a JSON-safe Go value.
// INT64 is kept as json.Number so values beyond 2^53 never lose precision.
func decodeValue(t *sppb.Type, v *structpb.Value) (any, error) {
	if _, isNull := v.GetKind().(*structpb.Value_NullValue); isNull {
		return nil, nil
	}
	switch t.GetCode() {
	case sppb.TypeCode_BOOL:
		return v.GetBoolValue(), nil
	case sppb.TypeCode_INT64, sppb.TypeCode_ENUM:
		return json.Number(v.GetStringValue()), nil
	case sppb.TypeCode_FLOAT64, sppb.TypeCode_FLOAT32:
		// NaN / Infinity / -Infinity arrive as strings and stay strings
		// because JSON has no representation for them.
		if s, ok := v.GetKind().(*structpb.Value_StringValue); ok {
			return s.StringValue, nil
		}
		return v.GetNumberValue(), nil
	case sppb.TypeCode_JSON:
		return json.RawMessage(v.GetStringValue()), nil
	case sppb.TypeCode_ARRAY:
		list := v.GetListValue().GetValues()
		out := make([]any, len(list))
		for i, elem := range list {
			decoded, err := decodeValue(t.GetArrayElementType(), elem)
			if err != nil {
				return nil, err
			}
			out[i] = decoded
		}
		return out, nil
	case sppb.TypeCode_STRUCT:
		fields := t.GetStructType().GetFields()
		list := v.GetListValue().GetValues()
		if len(fields) != len(list) {
			return nil, fmt.Errorf("struct value has %d fields but type has %d", len(list), len(fields))
		}
		out := make(map[string]any, len(fields))
		for i, field := range fields {
			decoded, err := decodeValue(field.GetType(), list[i])
			if err != nil {
				return nil, err
			}
			name := field.GetName()
			if name == "" {
				name = fmt.Sprintf("_field_%d", i)
			}
			out[name] = decoded
		}
		return out, nil
	default:
		// STRING, BYTES (base64), NUMERIC, TIMESTAMP, DATE, INTERVAL, UUID, PROTO
		return v.GetStringValue(), nil
	}
}
