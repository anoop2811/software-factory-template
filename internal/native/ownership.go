package native

import (
	"errors"
	"fmt"
	"strconv"
	"syscall"
)

// OwnershipError preserves the PID when termination or reap is unconfirmed.
// docs/adr/0068-go-native-harness-execution.md:44.
type OwnershipError struct {
	ProcessPID int
	evidence   *ownershipEvidence
}

func (e *OwnershipError) Error() string { return "native process ownership is unconfirmed" }

type ownershipEvidence struct {
	phase                          string
	signalErrno                    string
	reaped, exitConfirmed, drained bool
	group                          groupObservation
}

type groupObservation struct {
	state, errno string
}

// SafeDiagnostic describes cleanup without disclosing subprocess inputs or output.
// docs/adr/0102-parallel-go-qualification.md:118.
func (e *OwnershipError) SafeDiagnostic() string {
	if e.evidence == nil {
		return fmt.Sprintf("pid=%d reason=unavailable", e.ProcessPID)
	}
	evidence := e.evidence
	return fmt.Sprintf("pid=%d phase=%s signal_errno=%s reaped=%t exit_confirmed=%t drained=%t group_state=%s group_errno=%s",
		e.ProcessPID, evidence.phase, evidence.signalErrno, evidence.reaped,
		evidence.exitConfirmed, evidence.drained, evidence.group.state, evidence.group.errno)
}

func safeErrno(err error) string {
	if err == nil {
		return "none"
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return strconv.FormatInt(int64(errno), 10)
	}
	return "unknown"
}
