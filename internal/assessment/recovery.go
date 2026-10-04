package assessment

import (
	"context"
	"errors"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

// RecoveryInventory describes inert local evidence, never restore or deletion authority.
// docs/adr/0083-go-recovery-set-inspection.md:65.
type RecoveryInventory struct {
	SchemaVersion   int           `json:"schema_version"`
	RootStatus      string        `json:"root_status"`
	Complete        bool          `json:"complete"`
	Sets            []RecoverySet `json:"sets"`
	SetCount        int           `json:"set_count"`
	FileCount       int           `json:"file_count"`
	Bytes           int64         `json:"bytes"`
	Restorable      bool          `json:"restorable"`
	PruneAuthorized bool          `json:"prune_authorized"`
}

// RecoverySet reports one bounded inspection and a non-destructive next action.
type RecoverySet struct {
	Path            string `json:"path"`
	Classification  string `json:"classification"`
	Reason          string `json:"reason"`
	NextAction      string `json:"next_action"`
	FileCount       int    `json:"file_count"`
	Bytes           int64  `json:"bytes"`
	Held            *bool  `json:"held"`
	Restorable      bool   `json:"restorable"`
	PruneAuthorized bool   `json:"prune_authorized"`
}

// Status distinguishes inspection errors from preserved exceptions and valid observations.
func (r RecoveryInventory) Status() int {
	code := 0
	if r.RootStatus == "assessment_error" {
		return 1
	}
	if r.RootStatus != "absent" && r.RootStatus != "inspected" {
		code = 2
	}
	for _, set := range r.Sets {
		if set.Classification == "assessment_error" {
			return 1
		}
		if set.Classification != "integrity_checked" {
			code = 2
		}
	}
	return code
}

// InspectRecovery inspects existing storage without creating files or granting authority.
func InspectRecovery(ctx context.Context, root string) (RecoveryInventory, error) {
	return inspectRecovery(ctx, root, ops{})
}
func inspectRecovery(ctx context.Context, root string, operations ops) (RecoveryInventory, error) {
	result := RecoveryInventory{SchemaVersion: 1, RootStatus: "absent", Complete: true, Sets: []RecoverySet{}}
	if err := ctx.Err(); err != nil {
		return RecoveryInventory{}, err
	}
	chain, err := openRoot(ctx, root)
	if err != nil {
		return RecoveryInventory{}, err
	}
	defer chain.close()
	var pins recoveryPins
	defer func() { pins.close() }()
	parent := chain.last()
	for i, name := range []string{".factory", "backups"} {
		pin, class := openRecoveryDirectory(ctx, parent, name, i == 0, operations)
		if pin.file != nil {
			pins = append(pins, pin)
		}
		if err := ctx.Err(); err != nil {
			return RecoveryInventory{}, err
		}
		if class != "" {
			if class != "missing" {
				result.RootStatus = class
				result.Complete = false
			}
			result.invalidate(recoveryValidationClass(ctx, chain, pins, operations))
			return result, nil
		}
		parent = pin.file
	}
	entries, class := recoveryStorageEntries(ctx, parent, operations, &pins)
	if err := ctx.Err(); err != nil {
		return RecoveryInventory{}, err
	}
	result.RootStatus = "inspected"
	if class != "" {
		result.RootStatus = class
		result.Complete = false
	} else {
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return RecoveryInventory{}, err
			}
			row, err := inspectRecoverySet(ctx, parent, entry.Name(), operations)
			if err != nil {
				return RecoveryInventory{}, err
			}
			result.Sets = append(result.Sets, row)
			result.FileCount += row.FileCount
			result.Bytes += row.Bytes
		}
		result.SetCount = len(result.Sets)
	}
	if err := ctx.Err(); err != nil {
		return RecoveryInventory{}, err
	}
	result.invalidate(recoveryValidationClass(ctx, chain, pins, operations))
	return result, nil
}

// Invalid observations are discarded without losing an earlier I/O failure.
// docs/adr/0095-durable-live-publication.md:273.
func (r *RecoveryInventory) invalidate(class string) {
	if class == "" {
		return
	}
	r.RootStatus = recoveryInvalidationClass(r.Status(), class)
	r.Complete = false
	r.Sets, r.SetCount, r.FileCount, r.Bytes = []RecoverySet{}, 0, 0, 0
}

func recoveryInvalidationClass(status int, class string) string {
	if status == 1 || class == "assessment_error" {
		return "assessment_error"
	}
	return class
}

// Both retained metadata checks participate, with operational errors taking priority.
// docs/adr/0095-durable-live-publication.md:265.
func recoveryValidationClass(ctx context.Context, chain directories, pins recoveryPins, operations ops) string {
	class := ""
	for _, err := range []error{checkRecoveryRoot(ctx, chain, operations), pins.check(ctx, operations)} {
		if err != nil {
			next := "unsafe"
			if ErrorStatus(err) == 1 {
				next = "assessment_error"
			}
			class = mergeRecoveryClass(class, next)
		}
	}
	return class
}

type recoveryPin struct {
	parent, file *os.File
	name         string
	stat         unix.Stat_t
	missing      bool
}
type recoveryPins []recoveryPin

func (p recoveryPins) close() {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i].file != nil {
			_ = p[i].file.Close()
		}
	}
}
func (p recoveryPins) valid(options ...ops) bool {
	return p.check(context.Background(), metadataOperations(options)) == nil
}

func (p recoveryPins) check(ctx context.Context, operations ops) error {
	for _, pin := range p {
		if err := ctx.Err(); err != nil {
			return err
		}
		if pin.missing {
			if _, err := operations.lookup(pin.parent, pin.name); !errors.Is(err, unix.ENOENT) {
				if err != nil {
					return observationError(err, "cannot observe expected recovery absence")
				}
				return failure(2, "expected recovery absence changed")
			}
			continue
		}
		if pin.file != nil {
			current, err := operations.stat(pin.file)
			if err != nil {
				return observationError(err, "cannot observe retained recovery metadata")
			}
			if !recoveryUnchanged(pin.stat, current) {
				return failure(2, "retained recovery metadata changed")
			}
		}
		current, err := operations.lookup(pin.parent, pin.name)
		if err != nil {
			return observationError(err, "cannot observe named recovery metadata")
		}
		if !recoveryUnchanged(pin.stat, current) {
			return failure(2, "named recovery metadata changed")
		}
	}
	return nil
}
func recoveryUnchanged(a, b unix.Stat_t) bool { return unchanged(a, b) && a.Uid == b.Uid }
func recoveryDirectory(stat unix.Stat_t, factory bool) bool {
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR || int64(stat.Uid) != int64(os.Geteuid()) {
		return false
	}
	if factory {
		return stat.Mode&07000 == 0 && stat.Mode&0022 == 0
	}
	return stat.Mode&07777 == 0700
}
func recoveryFile(stat unix.Stat_t, limit int64) bool {
	return ordinary(stat) && stat.Size <= limit && stat.Mode&07777 == 0600 && int64(stat.Uid) == int64(os.Geteuid())
}
func recoveryOpen(operations ops, parent *os.File, name string, flags int) (*os.File, error) {
	if operations.open != nil {
		return operations.open(parent, name, flags|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC)
	}
	return openAt(parent, name, flags)
}
func openRecoveryDirectory(ctx context.Context, parent *os.File, name string, factory bool, operations ops) (recoveryPin, string) {
	if ctx.Err() != nil {
		return recoveryPin{}, "assessment_error"
	}
	before, err := named(parent, name)
	if err != nil {
		return recoveryPin{}, classification(err)
	}
	if !recoveryDirectory(before, factory) {
		return recoveryPin{}, "unsafe"
	}
	if ctx.Err() != nil {
		return recoveryPin{}, "assessment_error"
	}
	file, err := recoveryOpen(operations, parent, name, unix.O_RDONLY|unix.O_DIRECTORY)
	if err != nil {
		class := classification(err)
		if class == "missing" {
			class = "unsafe"
		}
		return recoveryPin{}, class
	}
	after, err := descriptor(file)
	if err != nil {
		_ = file.Close()
		return recoveryPin{}, "assessment_error"
	}
	if !recoveryUnchanged(before, after) {
		_ = file.Close()
		return recoveryPin{}, "unsafe"
	}
	return recoveryPin{parent: parent, file: file, name: name, stat: after}, ""
}
func recoveryEntries(ctx context.Context, file *os.File, limit int) ([]os.DirEntry, string) {
	if ctx.Err() != nil {
		return nil, "assessment_error"
	}
	entries, err := file.ReadDir(limit + 1)
	if len(entries) > limit {
		return nil, "limit_exceeded"
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, "assessment_error"
	}
	return entries, ""
}
func recoveryRow(name, class string, held *bool) RecoverySet {
	row := RecoverySet{Path: ".factory/backups/" + recoveryDisplayName(name), Classification: class, Held: held}
	switch class {
	case "missing":
		row.Reason = "recovery_path_absent"
		row.NextAction = "inspect_preserved_installation"
	case "integrity_checked":
		row.Reason = "transaction_authority_not_established"
		row.NextAction = "review_transaction_evidence"
	case "unrecognized":
		row.Reason = "unrecognized_recovery_record"
		row.NextAction = "inspect_preserved_set"
	case "incomplete":
		row.Reason = "recovery_content_incomplete"
		row.NextAction = "inspect_preserved_set"
	case "unsafe":
		row.Reason = "unsafe_recovery_path"
		row.NextAction = "resolve_unsafe_path"
	case "assessment_error":
		row.Reason = "cannot_inspect_recovery"
		row.NextAction = "retry_inspection"
	case "limit_exceeded":
		row.Reason = "recovery_limit_exceeded"
		row.NextAction = "reduce_inventory_scope"
	}
	return row
}

// Display identifiers preserve raw names without granting filesystem authority.
// docs/adr/0083-go-recovery-set-inspection.md:151.
func recoveryDisplayName(name string) string {
	const hex = "0123456789ABCDEF"
	var display strings.Builder
	for i := 0; i < len(name); i++ {
		b := name[i]
		if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '-' || b == '_' {
			display.WriteByte(b)
		} else {
			display.WriteByte('%')
			display.WriteByte(hex[b>>4])
			display.WriteByte(hex[b&15])
		}
	}
	return display.String()
}

// Recovery additionally binds root-chain permissions and ownership, while shared
// ancestor listing changes remain irrelevant to the inspected installation.
// docs/adr/0083-go-recovery-set-inspection.md:161.
func recoveryRootValid(chain directories, options ...ops) bool {
	return checkRecoveryRoot(context.Background(), chain, metadataOperations(options)) == nil
}

func checkRecoveryRoot(ctx context.Context, chain directories, operations ops) error {
	for i, entry := range chain {
		if err := ctx.Err(); err != nil {
			return err
		}
		matches := func(current unix.Stat_t) bool {
			if i == len(chain)-1 {
				return recoveryUnchanged(entry.identity, current)
			}
			return sameIdentity(entry.identity, current) && entry.identity.Mode == current.Mode && entry.identity.Uid == current.Uid
		}
		current, err := operations.stat(entry.file)
		if err != nil {
			return observationError(err, "cannot observe recovery root metadata")
		}
		if !matches(current) {
			return failure(2, "recovery root metadata changed")
		}
		var location unix.Stat_t
		if i == 0 {
			err = unix.Fstatat(unix.AT_FDCWD, entry.name, &location, unix.AT_SYMLINK_NOFOLLOW)
		} else {
			location, err = operations.lookup(chain[i-1].file, entry.name)
		}
		if err != nil {
			return observationError(err, "cannot observe named recovery root")
		}
		if !matches(location) {
			return failure(2, "named recovery root changed")
		}
	}
	return nil
}
