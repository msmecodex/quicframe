package main

import (
	"fmt"
	"os"

	"github.com/msmecodex/quicframe/internal/cli"
)

func main() {
	if err := cli.Main(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "quicframe:", err)
		os.Exit(1)
	}
}
