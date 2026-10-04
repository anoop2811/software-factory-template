package assessment

import "context"

// PublicationInventory observes inert records without granting live authority.
// docs/adr/0095-durable-live-publication.md:39.
type PublicationInventory struct {
	SchemaVersion   int                 `json:"schema_version"`
	Mode            string              `json:"mode"`
	RootStatus      string              `json:"root_status"`
	Complete        bool                `json:"complete"`
	PendingStatus   string              `json:"pending_status"`
	Records         []PublicationRecord `json:"records"`
	RecordCount     int                 `json:"record_count"`
	Restorable      bool                `json:"restorable"`
	RollbackReady   bool                `json:"rollback_ready"`
	ActivationReady bool                `json:"activation_ready"`
	Applicable      bool                `json:"applicable"`
	PruneAuthorized bool                `json:"prune_authorized"`
}

// PublicationRecord describes one bounded record, never reconstructed ownership.
// docs/adr/0095-durable-live-publication.md:43.
type PublicationRecord struct {
	MigrationID    string `json:"migration_id"`
	OperationID    string `json:"operation_id"`
	Path           string `json:"path"`
	Classification string `json:"classification"`
	Phase          string `json:"phase"`
	Outcome        string `json:"outcome"`
	NextAction     string `json:"next_action"`
}

// Status distinguishes operational observations from preserved blockers.
// docs/adr/0095-durable-live-publication.md:44.
func (r PublicationInventory) Status() int {
	if r.RootStatus == "assessment_error" || r.PendingStatus == "assessment_error" {
		return 1
	}
	status := 0
	for _, record := range r.Records {
		if record.Classification == "assessment_error" {
			return 1
		}
		if record.Classification != "record_checked" || record.Outcome == "pending" {
			status = 2
		}
	}
	if !r.Complete || r.PendingStatus != "absent" || r.RootStatus != "inspected" && r.RootStatus != "absent" {
		status = 2
	}
	return status
}

// BeginDurablePublication reserves durable pending before local Git queries.
// docs/adr/0095-durable-live-publication.md:25.
func BeginDurablePublication(ctx context.Context, root string, request PublicationRequest, environment map[string]string) (*Publication, error) {
	return beginDurablePublication(ctx, root, request, environment, publicationOps{})
}

func beginDurablePublication(ctx context.Context, root string, request PublicationRequest, environment map[string]string, operations publicationOps) (*Publication, error) {
	return constructPublication(ctx, root, request, operations, &durableSetup{environment: environment})
}

// Finish completes only this durable handle's actual owned forward image.
// docs/adr/0095-durable-live-publication.md:29.
func (p *Publication) Finish(ctx context.Context) error {
	if p == nil || p.state == nil {
		return failure(2, "publication handle unavailable")
	}
	return p.state.finish(ctx)
}

func (p *publicationState) finish(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.finished || p.journal == nil || p.journal.direction == "reverse" {
		return failure(2, "publication cannot finish forward")
	}
	if err := p.check(ctx); err != nil {
		return p.failed(err)
	}
	if !p.afterPublished || p.undoPublished {
		return failure(2, "publication has no owned forward image")
	}
	// An invalid Finish cannot latch authority; a valid direction precedes writes.
	// docs/adr/0095-durable-live-publication.md:144.
	p.journal.direction = "forward"
	if err := p.w.sync(ctx, p.current); err != nil {
		return p.failed(err)
	}
	if err := p.w.sync(ctx, p.parent); err != nil {
		return p.failed(err)
	}
	record := p.journal.record
	identity := assetIdentity(p.current.stat)
	record.Direction, record.Phase, record.Outcome = "forward", "forward_completed", "forward_completed"
	record.ForwardIdentity, record.CandidateIdentity, record.RestoredIdentity = &identity, nil, nil
	if err := p.writeRecord(ctx, record); err != nil {
		return p.failed(err)
	}
	if err := p.removePending(ctx); err != nil {
		return p.failed(err)
	}
	p.finished, p.lastError = true, nil
	return nil
}

// InspectPublications reads bounded inert evidence without locks or processes.
// docs/adr/0095-durable-live-publication.md:34.
func InspectPublications(ctx context.Context, root string) (PublicationInventory, error) {
	return inspectPublications(ctx, root, ops{})
}
