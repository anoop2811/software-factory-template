package main

import (
	"context"
	"os"

	"github.com/anoop2811/software-factory-template/internal/bridge"
	"github.com/anoop2811/software-factory-template/internal/cli"
)

func main() {
	// Candidate sourceable calls use an explicit private protocol; ordinary
	// invocation retains its public dispatcher. docs/DECISION_LOG.md:1960.
	if os.Getenv("FACTORY_BRIDGE_PROTOCOL") != "" {
		os.Exit(bridge.Run(context.Background(), os.Args[1:]))
	}
	os.Exit(cli.Run(context.Background(), os.Args[1:]))
}
