// Command yak renders configuration files from templates written in the yak
// language.
package main

import (
	"os"

	"github.com/daluz/yak/internal/cli"
)

func main() {
	os.Exit(cli.Main())
}
