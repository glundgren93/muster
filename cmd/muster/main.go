package main

import (
	"os"

	"github.com/glundgren93/muster/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
