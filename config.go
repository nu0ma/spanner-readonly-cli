package main

import (
	"cmp"
	"fmt"
)

type Config struct {
	Project  string
	Instance string
	Database string
	// Endpoint, when set, points at a Spanner Omni or other self-hosted
	// deployment and switches the client to unauthenticated plaintext gRPC.
	Endpoint string
}

func (c Config) DatabasePath() string {
	return fmt.Sprintf("projects/%s/instances/%s/databases/%s", c.Project, c.Instance, c.Database)
}

// resolveConfig fills each field from the flag value, falling back to the
// environment variables used by spanner-readonly-mcp.
func resolveConfig(project, instance, database, endpoint string, getenv func(string) string) (Config, error) {
	cfg := Config{
		Project:  cmp.Or(project, getenv("SPANNER_PROJECT")),
		Instance: cmp.Or(instance, getenv("SPANNER_INSTANCE")),
		Database: cmp.Or(database, getenv("SPANNER_DATABASE")),
		Endpoint: cmp.Or(endpoint, getenv("SPANNER_ENDPOINT")),
	}
	var missing []string
	if cfg.Project == "" {
		missing = append(missing, "--project / SPANNER_PROJECT")
	}
	if cfg.Instance == "" {
		missing = append(missing, "--instance / SPANNER_INSTANCE")
	}
	if cfg.Database == "" {
		missing = append(missing, "--database / SPANNER_DATABASE")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required configuration: %v", missing)
	}
	return cfg, nil
}
