package main

import (
	"os"

	"github.com/nu0ma/spanner-readonly-cli/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}
