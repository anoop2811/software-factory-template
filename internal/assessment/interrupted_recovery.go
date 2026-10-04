package assessment

import "context"

// InterruptedRecoveryRequest requires independently known bytes and fresh consent.
// docs/adr/0096-interrupted-publication-recovery.md:28.
type InterruptedRecoveryRequest struct {
	MigrationID        string
	OperationID        string
	Path               string
	Direction          string
	Replacement        []byte
	AfterReference     Observation
	UnbridgedQuiescent bool
}

// InterruptedRecoveryProposal describes current evidence without granting authority.
// docs/adr/0096-interrupted-publication-recovery.md:75.
type InterruptedRecoveryProposal struct {
	SchemaVersion       int           `json:"schema_version"`
	Mode                string        `json:"mode"`
	Scope               string        `json:"scope"`
	Purpose             string        `json:"purpose"`
	MigrationID         string        `json:"migration_id"`
	OperationID         string        `json:"operation_id"`
	Path                string        `json:"path"`
	Direction           string        `json:"direction"`
	CurrentImage        string        `json:"current_image"`
	ProposalDigest      string        `json:"proposal_digest"`
	RootIdentity        RootIdentity  `json:"root_identity"`
	Current             AdoptionAsset `json:"current"`
	Record              Observation   `json:"record"`
	RecordIdentity      AssetIdentity `json:"record_identity"`
	PendingIdentity     AssetIdentity `json:"pending_identity"`
	Recovery            RecoverySet   `json:"recovery"`
	StateCompatibility  string        `json:"state_compatibility"`
	UnbridgedQuiescence string        `json:"unbridged_quiescence"`
	Restorable          bool          `json:"restorable"`
	RollbackReady       bool          `json:"rollback_ready"`
	ActivationReady     bool          `json:"activation_ready"`
	Applicable          bool          `json:"applicable"`
	PruneAuthorized     bool          `json:"prune_authorized"`
}

// InterruptedRecovery is opaque; value aliases will share the private lifecycle.
// docs/adr/0096-interrupted-publication-recovery.md:63.
type InterruptedRecovery struct{ state *publicationState }

// ProposeInterruptedRecovery is unsupported during interface qualification.
// docs/adr/0096-interrupted-publication-recovery.md:38.
func ProposeInterruptedRecovery(ctx context.Context, root string, request InterruptedRecoveryRequest) (InterruptedRecoveryProposal, error) {
	return proposeInterruptedRecovery(ctx, root, request, ops{})
}

func proposeInterruptedRecovery(ctx context.Context, root string, request InterruptedRecoveryRequest, operations ops) (InterruptedRecoveryProposal, error) {
	return InterruptedRecoveryProposal{}, failure(2, "interrupted publication recovery is not implemented")
}

// BeginInterruptedRecovery is unsupported during interface qualification.
// docs/adr/0096-interrupted-publication-recovery.md:39.
func BeginInterruptedRecovery(ctx context.Context, root string, request InterruptedRecoveryRequest, confirmation string, environment map[string]string) (*InterruptedRecovery, error) {
	return beginInterruptedRecovery(ctx, root, request, confirmation, environment, publicationOps{})
}

func beginInterruptedRecovery(ctx context.Context, root string, request InterruptedRecoveryRequest, confirmation string, environment map[string]string, operations publicationOps) (*InterruptedRecovery, error) {
	return nil, failure(2, "interrupted publication recovery is not implemented")
}

// Complete is unsupported during interface qualification.
// docs/adr/0096-interrupted-publication-recovery.md:40.
func (r *InterruptedRecovery) Complete(ctx context.Context) error {
	return failure(2, "interrupted publication recovery is not implemented")
}

// Close is unsupported during interface qualification.
// docs/adr/0096-interrupted-publication-recovery.md:41.
func (r *InterruptedRecovery) Close(ctx context.Context) error {
	return failure(2, "interrupted publication recovery is not implemented")
}
