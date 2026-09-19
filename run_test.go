package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func runCLI(t *testing.T, args []string, env map[string]string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, &stdout, &stderr, envFrom(env))
	return code, stdout.String(), stderr.String()
}

func TestRunUsageErrors(t *testing.T) {
	cases := []struct {
		name string
		req  []string
		want string
	}{
		{
			name: "missing command",
			want: "a command is required",
		},
		{
			name: "unknown command",
			req:  []string{"drop"},
			want: "unknown command",
		},
		{
			name: "unknown flag",
			req:  []string{"query", "SELECT 1", "--unknown"},
			want: "flag provided but not defined",
		},
		{
			name: "invalid flag value",
			req:  []string{"query", "SELECT 1", "--max-rows", "abc"},
			want: "invalid value",
		},
		{
			name: "negative row limit",
			req:  []string{"query", "SELECT 1", "--max-rows", "-1"},
			want: "--max-rows must be zero or greater",
		},
		{
			name: "missing SQL",
			req:  []string{"query"},
			want: "exactly one SQL argument",
		},
		{
			name: "missing table",
			req:  []string{"describe"},
			want: "exactly one table argument",
		},
		{
			name: "unexpected positional argument",
			req:  []string{"tables", "Users"},
			want: "does not accept positional arguments",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(t, tc.req, nil)
			if code != 2 || stdout != "" {
				t.Fatalf("code=%d stdout=%q stderr=%s", code, stdout, stderr)
			}
			var got errorResult
			if err := json.Unmarshal([]byte(stderr), &got); err != nil {
				t.Fatalf("stderr must be a single JSON error: %v: %s", err, stderr)
			}
			if got.Code != "InvalidArgument" || got.Retryable || !strings.Contains(got.Error, tc.want) {
				t.Fatalf("unexpected error: %+v", got)
			}
		})
	}
}

func TestRunMissingConfigIsJSONError(t *testing.T) {
	code, _, stderr := runCLI(t, []string{"tables"}, nil)
	if code == 0 {
		t.Fatal("want non-zero exit code")
	}
	var got errorResult
	if err := json.Unmarshal([]byte(stderr), &got); err != nil {
		t.Fatalf("stderr must be JSON: %v", err)
	}
	if got.Code != "FailedPrecondition" || got.Retryable || !strings.Contains(got.Error, "SPANNER_PROJECT") {
		t.Fatalf("unexpected error: %+v", got)
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

func TestRunSubcommandHelp(t *testing.T) {
	code, stdout, stderr := runCLI(t, []string{"query", "--help"}, nil)
	if code != 0 || stderr != "" || !strings.Contains(stdout, "max-rows") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestRunFlagsAfterPositionalArgs(t *testing.T) {
	// Agents habitually put flags last: spanner-readonly-cli query "SELECT 1" --param x=y
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

func TestRunVersion(t *testing.T) {
	for _, arg := range []string{"version", "--version", "-v"} {
		code, stdout, _ := runCLI(t, []string{arg}, nil)
		if code != 0 {
			t.Fatalf("%s: exit code %d", arg, code)
		}
		if !strings.Contains(stdout, version) {
			t.Fatalf("%s: stdout should contain version %q: %s", arg, version, stdout)
		}
	}
}

func TestWriteError(t *testing.T) {
	cases := []struct {
		name string
		req  error
		want errorResult
	}{
		{
			name: "permission denied",
			req:  spanner.ToSpannerError(status.Error(codes.PermissionDenied, "access denied")),
			want: errorResult{
				Code: "PermissionDenied",
			},
		},
		{
			name: "wrapped transient Spanner error",
			req:  fmt.Errorf("query failed: %w", spanner.ToSpannerError(status.Error(codes.Unavailable, "service unavailable"))),
			want: errorResult{
				Code:      "Unavailable",
				Retryable: true,
			},
		},
		{
			name: "deadline exceeded",
			req:  fmt.Errorf("query failed: %w", context.DeadlineExceeded),
			want: errorResult{
				Code:      "DeadlineExceeded",
				Retryable: true,
			},
		},
		{
			name: "canceled",
			req:  context.Canceled,
			want: errorResult{
				Code: "Canceled",
			},
		},
		{
			name: "unknown local error",
			req:  errors.New("output failed"),
			want: errorResult{
				Code: "Unknown",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if code := writeError(&stderr, tc.req); code != 1 {
				t.Fatalf("exit code: got %d, want 1", code)
			}
			var got errorResult
			if err := json.Unmarshal(stderr.Bytes(), &got); err != nil {
				t.Fatalf("stderr must be JSON: %v", err)
			}
			want := tc.want
			want.Error = tc.req.Error()
			if diff := cmp.Diff(want, got); diff != "" {
				t.Fatalf("error mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
