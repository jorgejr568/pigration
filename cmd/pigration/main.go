// Command pigration is the CLI for the pigration migration engine.
package main

import (
	"fmt"
	"os"

	"github.com/jorgejr568/pigration/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "pigration: %v\n", err)
		os.Exit(1)
	}
}
