package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const defaultTimeout = 30 * time.Second

const usage = `Usage: spanner-ro <command> [flags]

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

SPANNER_EMULATOR_HOST is honored for local development.
Output is a single JSON object: {"columns": [...], "rows": [...], "rowCount": N}
`

// stringSlice collects repeated flag values.
type stringSlice []string

func (s *stringSlice) String() string     { return fmt.Sprint(*s) }
func (s *stringSlice) Set(v string) error { *s = append(*s, v); return nil }

func Run(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	command := args[0]
	if command == "--help" || command == "-h" || command == "help" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if command == "version" || command == "--version" || command == "-v" {
		fmt.Fprintln(stdout, "spanner-ro version "+version)
		return 0
	}

	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	project := fs.String("project", "", "GCP project ID (SPANNER_PROJECT)")
	instance := fs.String("instance", "", "Spanner instance ID (SPANNER_INSTANCE)")
	database := fs.String("database", "", "Spanner database ID (SPANNER_DATABASE)")
	endpoint := fs.String("endpoint", "", "Spanner Omni endpoint, e.g. localhost:15000 (SPANNER_ENDPOINT)")
	timeout := fs.Duration("timeout", defaultTimeout, "query timeout")
	var paramFlags stringSlice
	var tableFilter string
	switch command {
	case "query":
		fs.Var(&paramFlags, "param", "query parameter as name=value (repeatable)")
	case "indexes":
		fs.StringVar(&tableFilter, "table", "", "filter indexes by table name")
	case "tables", "describe":
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", command, usage)
		return 2
	}
	positional, err := parseInterleaved(fs, args[1:])
	if err != nil {
		return 2
	}

	var stmt spanner.Statement
	switch command {
	case "query":
		if len(positional) != 1 {
			return writeError(stderr, fmt.Errorf("query requires exactly one SQL argument"))
		}
		params, err := parseParams(paramFlags)
		if err != nil {
			return writeError(stderr, err)
		}
		stmt = spanner.Statement{SQL: positional[0], Params: params}
	case "tables":
		stmt = tablesStatement()
	case "describe":
		if len(positional) != 1 {
			return writeError(stderr, fmt.Errorf("describe requires exactly one table argument"))
		}
		stmt = describeStatement(positional[0])
	case "indexes":
		stmt = indexesStatement(tableFilter)
	}

	cfg, err := resolveConfig(*project, *instance, *database, *endpoint, getenv)
	if err != nil {
		return writeError(stderr, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	result, err := executeReadOnly(ctx, cfg, stmt)
	if err != nil {
		return writeError(stderr, err)
	}

	out, err := marshalResult(result)
	if err != nil {
		return writeError(stderr, err)
	}
	fmt.Fprintln(stdout, string(out))
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
func executeReadOnly(ctx context.Context, cfg Config, stmt spanner.Statement) (Result, error) {
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
	// Marshaling a map[string]string cannot fail.
	msg, _ := marshalJSON(map[string]string{"error": err.Error()})
	fmt.Fprintln(stderr, string(msg))
	return 1
}
