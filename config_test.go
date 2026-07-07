package main

import (
	"strings"
	"testing"
)

func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestResolveConfigFlagsWinOverEnv(t *testing.T) {
	env := envFrom(map[string]string{
		"SPANNER_PROJECT":  "p-env",
		"SPANNER_INSTANCE": "i-env",
		"SPANNER_DATABASE": "d-env",
	})
	cfg, err := resolveConfig("p-flag", "", "d-flag", "", env)
	if err != nil {
		t.Fatalf("resolveConfig error: %v", err)
	}
	if cfg.Project != "p-flag" || cfg.Instance != "i-env" || cfg.Database != "d-flag" {
		t.Fatalf("got %+v", cfg)
	}
}

func TestResolveConfigEndpoint(t *testing.T) {
	env := envFrom(map[string]string{
		"SPANNER_PROJECT":  "p",
		"SPANNER_INSTANCE": "i",
		"SPANNER_DATABASE": "d",
		"SPANNER_ENDPOINT": "localhost:15000",
	})
	cfg, err := resolveConfig("", "", "", "", env)
	if err != nil {
		t.Fatalf("resolveConfig error: %v", err)
	}
	if cfg.Endpoint != "localhost:15000" {
		t.Fatalf("endpoint from env: got %q", cfg.Endpoint)
	}
	cfg, err = resolveConfig("", "", "", "other:9999", env)
	if err != nil {
		t.Fatalf("resolveConfig error: %v", err)
	}
	if cfg.Endpoint != "other:9999" {
		t.Fatalf("flag should win over env: got %q", cfg.Endpoint)
	}
}

func TestResolveConfigMissingValues(t *testing.T) {
	_, err := resolveConfig("", "i", "d", "", envFrom(nil))
	if err == nil || !strings.Contains(err.Error(), "SPANNER_PROJECT") {
		t.Fatalf("want error mentioning SPANNER_PROJECT, got %v", err)
	}
	_, err = resolveConfig("p", "i", "", "", envFrom(nil))
	if err == nil || !strings.Contains(err.Error(), "SPANNER_DATABASE") {
		t.Fatalf("want error mentioning SPANNER_DATABASE, got %v", err)
	}
}

func TestConfigDatabasePath(t *testing.T) {
	cfg := Config{Project: "p", Instance: "i", Database: "d"}
	want := "projects/p/instances/i/databases/d"
	if got := cfg.DatabasePath(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
