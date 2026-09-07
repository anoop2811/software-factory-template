package main

import (
	"context"
	"os"

	"github.com/anoop2811/software-factory-template/internal/cli"
)

func main() {
	os.Exit(cli.Run(context.Background(), os.Args[1:]))
}
