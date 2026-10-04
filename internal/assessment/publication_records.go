package assessment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"reflect"
	"strconv"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/jsonvalue"
)

const publicationNamespace = ".publications"
const publicationRecordLimit = 64 << 10

// Records contain descriptive identities, never imported publication authority.
// docs/adr/0095-durable-live-publication.md:101.
type publicationRecordData struct {
	SchemaVersion     int            `json:"schema_version"`
	MigrationID       string         `json:"migration_id"`
	OperationID       string         `json:"operation_id"`
	Path              string         `json:"path"`
	RecoveryIdentity  RootIdentity   `json:"recovery_identity"`
	Before            Observation    `json:"before"`
	After             Observation    `json:"after"`
	OriginalIdentity  AssetIdentity  `json:"original_identity"`
	ForwardIdentity   *AssetIdentity `json:"forward_identity"`
	CandidateIdentity *AssetIdentity `json:"candidate_identity"`
	RestoredIdentity  *AssetIdentity `json:"restored_identity"`
	Direction         string         `json:"direction"`
	Phase             string         `json:"phase"`
	Outcome           string         `json:"outcome"`
}

func publicationObservation(data []byte, mode string) Observation {
	hash := sha256.Sum256(data)
	return Observation{SHA256: hex.EncodeToString(hash[:]), Mode: mode, Bytes: int64(len(data))}
}

func publicationTerminal(phase string) bool {
	return phase == "restored" || phase == "aborted" || phase == "forward_completed"
}

func publicationNumber(value string, signed bool) bool {
	if signed {
		number, err := strconv.ParseInt(value, 10, 64)
		return err == nil && strconv.FormatInt(number, 10) == value
	}
	number, err := strconv.ParseUint(value, 10, 64)
	return err == nil && strconv.FormatUint(number, 10) == value
}

func publicationIdentity(identity AssetIdentity, observation Observation) bool {
	return publicationNumber(identity.Device, true) && publicationNumber(identity.Inode, false) && identity.Inode != "0" &&
		identity.Type == "regular" && identity.Mode == observation.Mode && identity.Bytes == observation.Bytes &&
		publicationNumber(identity.MtimeSeconds, true) && publicationNumber(identity.CtimeSeconds, true) &&
		identity.MtimeNanoseconds >= 0 && identity.MtimeNanoseconds < 1_000_000_000 && identity.CtimeNanoseconds >= 0 && identity.CtimeNanoseconds < 1_000_000_000
}

func validPublicationRecord(record publicationRecordData, id string) bool {
	if record.SchemaVersion != 1 || record.MigrationID != id || !recoveryID(id) || !recoveryID(record.OperationID) ||
		!publicationNumber(record.RecoveryIdentity.Device, true) || !publicationNumber(record.RecoveryIdentity.Inode, false) || record.RecoveryIdentity.Inode == "0" {
		return false
	}
	selected, err := selectedReferences([]string{record.Path})
	if err != nil || record.Before != selected[0].Reference || !validProposalDigest(record.After.SHA256) ||
		record.After.Bytes < 0 || record.After.Bytes > assetLimit || record.After.Mode != record.Before.Mode ||
		!publicationIdentity(record.OriginalIdentity, record.Before) {
		return false
	}
	if record.ForwardIdentity != nil && !publicationIdentity(*record.ForwardIdentity, record.After) {
		return false
	}
	if record.RestoredIdentity != nil && !publicationIdentity(*record.RestoredIdentity, record.Before) {
		return false
	}
	if record.Direction != "forward" && record.Direction != "reverse" {
		return false
	}
	if publicationTerminal(record.Phase) {
		if record.Outcome != record.Phase || record.CandidateIdentity != nil {
			return false
		}
	} else if record.Outcome != "pending" {
		return false
	}
	switch record.Phase {
	case "prepared":
		return record.Direction == "forward" && record.ForwardIdentity == nil && record.CandidateIdentity == nil && record.RestoredIdentity == nil
	case "forward_prepared":
		return record.Direction == "forward" && record.ForwardIdentity != nil && record.CandidateIdentity != nil &&
			publicationIdentity(*record.CandidateIdentity, record.After) && record.RestoredIdentity == nil
	case "forward_published", "forward_completed":
		return record.Direction == "forward" && record.ForwardIdentity != nil && record.CandidateIdentity == nil && record.RestoredIdentity == nil
	case "reverse_prepared":
		return record.Direction == "reverse" && record.ForwardIdentity != nil && record.CandidateIdentity != nil &&
			publicationIdentity(*record.CandidateIdentity, record.Before) && record.RestoredIdentity == nil
	case "restored":
		return record.Direction == "reverse" && record.ForwardIdentity != nil && record.RestoredIdentity != nil
	case "aborted":
		return record.Direction == "reverse" && record.RestoredIdentity == nil
	default:
		return false
	}
}

// The existing decoder rejects duplicate keys before strict typed decoding.
// docs/adr/0095-durable-live-publication.md:101.
func parsePublicationRecord(ctx context.Context, data []byte, id string) (publicationRecordData, bool) {
	if len(data) > publicationRecordLimit || !utf8.Valid(data) {
		return publicationRecordData{}, false
	}
	value, err := jsonvalue.DecodeUnique(ctx, data)
	if err != nil || !publicationShape(value, reflect.TypeFor[publicationRecordData]()) {
		return publicationRecordData{}, false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record publicationRecordData
	if decoder.Decode(&record) != nil || decoder.Decode(new(any)) != io.EOF || ctx.Err() != nil || !validPublicationRecord(record, id) {
		return publicationRecordData{}, false
	}
	return record, true
}

// Derive required keys and scalar types from the closed record's own struct.
// Null identity values remain subject to the phase-specific validation above.
// docs/adr/0095-durable-live-publication.md:204.
func publicationShape(value any, shape reflect.Type) bool {
	if shape.Kind() == reflect.Pointer {
		return value == nil || publicationShape(value, shape.Elem())
	}
	switch shape.Kind() {
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok || len(object) != shape.NumField() {
			return false
		}
		for i := 0; i < shape.NumField(); i++ {
			field := shape.Field(i)
			child, present := object[field.Tag.Get("json")]
			if !present || !publicationShape(child, field.Type) {
				return false
			}
		}
		return true
	case reflect.String:
		_, ok := value.(string)
		return ok
	case reflect.Int, reflect.Int64:
		_, ok := value.(json.Number)
		return ok
	default:
		return false
	}
}

func encodePublicationRecord(ctx context.Context, record publicationRecordData) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validPublicationRecord(record, record.MigrationID) {
		return nil, failure(1, "cannot encode publication record")
	}
	data, err := json.Marshal(record)
	if err != nil || len(data)+1 > publicationRecordLimit {
		return nil, failure(1, "cannot encode bounded publication record")
	}
	return append(data, '\n'), nil
}
