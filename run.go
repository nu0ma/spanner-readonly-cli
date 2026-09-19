package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const defaultTimeout = 30 * time.Second
const defaultMaxRows = 100

const usage = `Usage: spanner-readonly-cli <command> [flags]

Read-only Cloud Spanner CLI. Every query runs inside a read-only snapshot
transaction, so writes are impossible by construction.

Commands:
  query <sql>       Execute a SELECT statement
                      --param name=value   bind a STRING parameter (repeatable)
  tables            List user tables
  describe <table>  Show column definitions of a table
  indexes           List indexes
                      --table <name>       filter by table

Connection flags (fall back to environment variables):
  --project    GCP project ID        (SPANNER_PROJECT)
  --instance   Spanner instance ID   (SPANNER_INSTANCE)
  --database   Spanner database ID   (SPANNER_DATABASE)
  --endpoint   Spanner Omni endpoint (SPANNER_ENDPOINT), e.g. localhost:15000
               connects without authentication over plaintext gRPC;
               project and instance are both "default" on Omni
  --timeout    query timeout, e.g. 30s, 2m (default 30s)
  --max-rows   maximum returned rows (default 100; 0 means unlimited)

SPANNER_EMULATOR_HOST is honored for local development.
Output is a single JSON object: {"columns": [...], "rows": [...], "rowCount": N, "truncated": false}
`

// stringSlice collects repeated flag values.
type stringSlice []string

func (s *stringSlice) String() string     { return fmt.Sprint(*s) }
func (s *stringSlice) Set(v string) error { *s = append(*s, v); return nil }

func Run(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	if len(args) == 0 {
		return writeUsageError(stderr, fmt.Errorf("a command is required; use --help for usage"))
	}
	command := args[0]
	if command == "--help" || command == "-h" || command == "help" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if command == "version" || command == "--version" || command == "-v" {
		fmt.Fprintln(stdout, "spanner-readonly-cli version "+version)
		return 0
	}

	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	project := fs.String("project", "", "GCP project ID (SPANNER_PROJECT)")
	instance := fs.String("instance", "", "Spanner instance ID (SPANNER_INSTANCE)")
	database := fs.String("database", "", "Spanner database ID (SPANNER_DATABASE)")
	endpoint := fs.String("endpoint", "", "Spanner Omni endpoint, e.g. localhost:15000 (SPANNER_ENDPOINT)")
	timeout := fs.Duration("timeout", defaultTimeout, "query timeout")
	maxRows := fs.Int("max-rows", defaultMaxRows, "maximum returned rows (0 means unlimited)")
	var paramFlags stringSlice
	var tableFilter string
	switch command {
	case "query":
		fs.Var(&paramFlags, "param", "query parameter as name=value (repeatable)")
	case "indexes":
		fs.StringVar(&tableFilter, "table", "", "filter indexes by table name")
	case "tables", "describe":
	default:
		return writeUsageError(stderr, fmt.Errorf("unknown command %q; use --help for usage", command))
	}
	positional, err := parseInterleaved(fs, args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(stdout, "Usage: spanner-readonly-cli %s [flags]\n", command)
			fs.SetOutput(stdout)
			fs.PrintDefaults()
			return 0
		}
		return writeUsageError(stderr, err)
	}
	if *maxRows < 0 {
		return writeUsageError(stderr, fmt.Errorf("--max-rows must be zero or greater"))
	}

	var stmt spanner.Statement
	switch command {
	case "query":
		if len(positional) != 1 {
			return writeUsageError(stderr, fmt.Errorf("query requires exactly one SQL argument"))
		}
		params, err := parseParams(paramFlags)
		if err != nil {
			return writeUsageError(stderr, err)
		}
		stmt = spanner.Statement{SQL: positional[0], Params: params}
	case "tables":
		if len(positional) != 0 {
			return writeUsageError(stderr, fmt.Errorf("tables does not accept positional arguments"))
		}
		stmt = tablesStatement()
	case "describe":
		if len(positional) != 1 {
			return writeUsageError(stderr, fmt.Errorf("describe requires exactly one table argument"))
		}
		stmt = describeStatement(positional[0])
	case "indexes":
		if len(positional) != 0 {
			return writeUsageError(stderr, fmt.Errorf("indexes does not accept positional arguments; use --table"))
		}
		stmt = indexesStatement(tableFilter)
	}

	cfg, err := resolveConfig(*project, *instance, *database, *endpoint, getenv)
	if err != nil {
		return writeError(stderr, status.Error(codes.FailedPrecondition, err.Error()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	result, err := executeReadOnly(ctx, cfg, stmt, *maxRows)
	if err != nil {
		return writeError(stderr, err)
	}

	out, err := marshalResult(result)
	if err != nil {
		return writeError(stderr, err)
	}
	if _, err := fmt.Fprintln(stdout, string(out)); err != nil {
		return writeError(stderr, err)
	}
	return 0
}

// parseInterleaved allows flags to appear after positional arguments
// (e.g. `query "SELECT ..." --param x=y`), which the flag package alone
// does not: it stops at the first non-flag argument.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

// executeReadOnly runs stmt inside a single-use read-only snapshot
// transaction (client.Single). spanner.ReadOnlyTransaction exposes no write
// methods, so writes are impossible regardless of the SQL text; DML/DDL is
// rejected by the server.
func executeReadOnly(ctx context.Context, cfg Config, stmt spanner.Statement, maxRows int) (Result, error) {
	client, err := newSpannerClient(ctx, cfg)
	if err != nil {
		return Result{}, fmt.Errorf("failed to create Spanner client: %w", err)
	}
	defer client.Close()

	iter := client.Single().Query(ctx, stmt)
	defer iter.Stop()

	var result Result
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return Result{}, err
		}
		// Read one extra row to distinguish an exact limit from a partial result.
		if maxRows > 0 && len(result.Rows) == maxRows {
			result.Truncated = true
			break
		}
		columns, m, err := rowToMap(row)
		if err != nil {
			return Result{}, err
		}
		if result.Columns == nil {
			result.Columns = columns
		}
		result.Rows = append(result.Rows, m)
	}
	result.RowCount = len(result.Rows)
	return result, nil
}

// newSpannerClient connects with Application Default Credentials by default.
// With cfg.Endpoint set it targets a Spanner Omni (or other self-hosted)
// deployment instead: unauthenticated plaintext gRPC, as TLS is not
// supported in the current Omni preview.
func newSpannerClient(ctx context.Context, cfg Config) (*spanner.Client, error) {
	if cfg.Endpoint == "" {
		return spanner.NewClient(ctx, cfg.DatabasePath())
	}
	return spanner.NewClientWithConfig(ctx, cfg.DatabasePath(),
		spanner.ClientConfig{IsExperimentalHost: true},
		omniClientOptions(cfg.Endpoint)...)
}

func omniClientOptions(endpoint string) []option.ClientOption {
	return []option.ClientOption{
		option.WithEndpoint(endpoint),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	}
}

func writeError(stderr io.Writer, err error) int {
	code := spanner.ErrCode(err)
	if errors.Is(err, context.DeadlineExceeded) {
		code = codes.DeadlineExceeded
	} else if errors.Is(err, context.Canceled) {
		code = codes.Canceled
	}
	// These read-only operations can be retried for transient service errors.
	retryable := code == codes.Unavailable || code == codes.Aborted || code == codes.DeadlineExceeded
	msg, _ := marshalJSON(errorResult{
		Error:     err.Error(),
		Code:      code.String(),
		Retryable: retryable,
	})
	fmt.Fprintln(stderr, string(msg))
	return 1
}

type errorResult struct {
	Error     string `json:"error"`
	Code      string `json:"code"`
	Retryable bool   `json:"retryable"`
}

func writeUsageError(stderr io.Writer, err error) int {
	writeError(stderr, status.Error(codes.InvalidArgument, err.Error()))
	return 2
}
