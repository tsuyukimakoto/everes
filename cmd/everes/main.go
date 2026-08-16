// Command everes は並行するAIコーディング作業のTask Context Manager。
package main

import (
	"os"

	"github.com/tsuyukimakoto/everes/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
