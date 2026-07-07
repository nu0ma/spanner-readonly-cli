package cli

import "fmt"

type Config struct {
	Project  string
	Instance string
	Database string
}

func (c Config) DatabasePath() string {
	return fmt.Sprintf("projects/%s/instances/%s/databases/%s", c.Project, c.Instance, c.Database)
}

// resolveConfig fills each field from the flag value, falling back to the
// environment variables used by spanner-readonly-mcp.
func resolveConfig(project, instance, database string, getenv func(string) string) (Config, error) {
	cfg := Config{
		Project:  firstNonEmpty(project, getenv("SPANNER_PROJECT")),
		Instance: firstNonEmpty(instance, getenv("SPANNER_INSTANCE")),
		Database: firstNonEmpty(database, getenv("SPANNER_DATABASE")),
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

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
