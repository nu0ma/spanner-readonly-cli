package main

import (
	"reflect"
	"testing"
)

func TestParseParams(t *testing.T) {
	got, err := parseParams([]string{"name=foo", "q=a=b"})
	if err != nil {
		t.Fatalf("parseParams error: %v", err)
	}
	want := map[string]any{"name": "foo", "q": "a=b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestParseParamsInvalid(t *testing.T) {
	if _, err := parseParams([]string{"noequals"}); err == nil {
		t.Fatal("want error for value without '='")
	}
	if _, err := parseParams([]string{"=v"}); err == nil {
		t.Fatal("want error for empty name")
	}
}
