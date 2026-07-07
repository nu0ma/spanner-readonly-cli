package cli

import (
	"fmt"
	"strings"
)

// parseParams turns repeated --param name=value pairs into Spanner query
// parameters. Values are always STRING typed; cast in SQL when needed,
// e.g. WHERE id = CAST(@id AS INT64).
func parseParams(pairs []string) (map[string]any, error) {
	params := make(map[string]any, len(pairs))
	for _, pair := range pairs {
		name, value, ok := strings.Cut(pair, "=")
		if !ok || name == "" {
			return nil, fmt.Errorf("invalid --param %q: expected name=value", pair)
		}
		params[name] = value
	}
	return params, nil
}
