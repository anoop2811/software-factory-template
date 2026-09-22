// Package native prepares and supervises already-admitted native harness runs.
package native

type Request struct{ Harness, Role, Model, Root, Prompt string }
type Plan struct {
	Harness, Role, Root string
	Argv                []string
	Stdin               string
	Environment         map[string]string
}
type Execution struct {
	Outcome                             string
	ExitCode                            *int
	ProcessPID                          int
	ElapsedSeconds                      float64
	Stdout                              []byte
	ExitConfirmed, OwnershipUnconfirmed bool
}
