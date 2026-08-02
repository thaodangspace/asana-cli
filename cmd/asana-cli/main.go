// Command asana-cli is a standalone CLI for Asana operations, ported from the
// pi-extensions Asana extension so any agent can invoke it from the shell.
package main

import (
	"os"

	"github.com/thaodangspace/asana-cli/cli"
)

func main() {
	os.Exit(cli.Execute())
}
