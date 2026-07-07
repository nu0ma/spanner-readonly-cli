package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
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

	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	project := fs.String("project", "", "GCP project ID (SPANNER_PROJECT)")
	instance := fs.String("instance", "", "Spanner instance ID (SPANNER_INSTANCE)")
	database := fs.String("database", "", "Spanner database ID (SPANNER_DATABASE)")
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

	stmt, err := buildStatement(command, positional, paramFlags, tableFilter)
	if err != nil {
		return writeError(stderr, err)
	}

	cfg, err := resolveConfig(*project, *instance, *database, getenv)
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

func buildStatement(command string, positional []string, paramFlags stringSlice, tableFilter string) (spanner.Statement, error) {
	switch command {
	case "query":
		if len(positional) != 1 {
			return spanner.Statement{}, fmt.Errorf("query requires exactly one SQL argument")
		}
		params, err := parseParams(paramFlags)
		if err != nil {
			return spanner.Statement{}, err
		}
		return spanner.Statement{SQL: positional[0], Params: params}, nil
	case "tables":
		return tablesStatement(), nil
	case "describe":
		if len(positional) != 1 {
			return spanner.Statement{}, fmt.Errorf("describe requires exactly one table argument")
		}
		return describeStatement(positional[0]), nil
	case "indexes":
		return indexesStatement(tableFilter), nil
	}
	return spanner.Statement{}, fmt.Errorf("unknown command %q", command)
}

// executeReadOnly runs stmt inside a single-use read-only snapshot
// transaction (client.Single). spanner.ReadOnlyTransaction exposes no write
// methods, so writes are impossible regardless of the SQL text; DML/DDL is
// rejected by the server.
func executeReadOnly(ctx context.Context, cfg Config, stmt spanner.Statement) (Result, error) {
	client, err := spanner.NewClient(ctx, cfg.DatabasePath())
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

func writeError(stderr io.Writer, err error) int {
	msg, marshalErr := marshalJSON(map[string]string{"error": err.Error()})
	if marshalErr != nil {
		fmt.Fprintf(stderr, `{"error":%q}`+"\n", err.Error())
		return 1
	}
	fmt.Fprintln(stderr, string(msg))
	return 1
}
