package main

import (
	"os"

	"github.com/gsid-nl/kdef/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
