package cli

import (
	"bytes"
	"strings"
	"testing"
)

func runCLI(t *testing.T, args []string, env map[string]string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, &stdout, &stderr, envFrom(env))
	return code, stdout.String(), stderr.String()
}

func TestRunNoArgsShowsUsage(t *testing.T) {
	code, _, stderr := runCLI(t, nil, nil)
	if code != 2 {
		t.Fatalf("exit code: got %d, want 2", code)
	}
	if !strings.Contains(stderr, "Usage") {
		t.Fatalf("stderr should show usage: %s", stderr)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	code, _, stderr := runCLI(t, []string{"drop"}, nil)
	if code != 2 || !strings.Contains(stderr, "unknown command") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
}

func TestRunQueryRequiresSQL(t *testing.T) {
	code, _, stderr := runCLI(t, []string{"query"}, map[string]string{
		"SPANNER_PROJECT": "p", "SPANNER_INSTANCE": "i", "SPANNER_DATABASE": "d",
	})
	if code == 0 || !strings.Contains(stderr, "SQL") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
}

func TestRunMissingConfigIsJSONError(t *testing.T) {
	code, _, stderr := runCLI(t, []string{"tables"}, nil)
	if code == 0 {
		t.Fatal("want non-zero exit code")
	}
	if !strings.Contains(stderr, `{"error":`) || !strings.Contains(stderr, "SPANNER_PROJECT") {
		t.Fatalf("stderr should be JSON error mentioning missing config: %s", stderr)
	}
}

func TestRunDescribeRequiresTable(t *testing.T) {
	code, _, stderr := runCLI(t, []string{"describe"}, map[string]string{
		"SPANNER_PROJECT": "p", "SPANNER_INSTANCE": "i", "SPANNER_DATABASE": "d",
	})
	if code == 0 || !strings.Contains(stderr, "table") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
}

func TestRunHelp(t *testing.T) {
	code, stdout, _ := runCLI(t, []string{"--help"}, nil)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0", code)
	}
	for _, cmd := range []string{"query", "tables", "describe", "indexes"} {
		if !strings.Contains(stdout, cmd) {
			t.Fatalf("usage should mention %q: %s", cmd, stdout)
		}
	}
}

func TestRunFlagsAfterPositionalArgs(t *testing.T) {
	// Agents habitually put flags last: spanner-ro query "SELECT 1" --param x=y
	// The --project flag after the positional arg must still be recognized,
	// so the error should be about the remaining missing config, not the SQL.
	code, _, stderr := runCLI(t, []string{"describe", "Users", "--project", "p"}, nil)
	if code == 0 {
		t.Fatal("want non-zero exit code")
	}
	if strings.Contains(stderr, "exactly one table") {
		t.Fatalf("flag after positional arg was treated as positional: %s", stderr)
	}
	if !strings.Contains(stderr, "SPANNER_INSTANCE") || strings.Contains(stderr, "SPANNER_PROJECT") {
		t.Fatalf("--project after positional should be honored: %s", stderr)
	}
}
