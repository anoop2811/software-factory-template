// Command worker-count is repository-private CI tooling, never installed.
package main

import (
	"os"

	"github.com/anoop2811/software-factory-template/internal/qualification"
)

func main() {
	environment := make(map[string]string)
	for _, key := range []string{"GO_RUNTIME_TEST_PROCS", "GO_RUNTIME_TEST_MAX_PROCS"} {
		if value, present := os.LookupEnv(key); present {
			environment[key] = value
		}
	}
	os.Exit(qualification.Run(os.Args[1:], environment, os.Stdout, os.Stderr))
}
