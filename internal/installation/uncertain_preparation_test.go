package installation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/installedlayout"
	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/transition"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

// These local component fixtures grant no archive trust, activation or real gate
// proof. Their small shell child only witnesses the actual native spawn/closure
// boundary; the installed-layout bytes are independently specified fixture data.
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:483
const preparationComponentChild = `#!/bin/sh
printf '%s\t%s\n' "$$" "$1" >> "${0}.executions"
printf 'CHILD_PID=%s ARG=%s\n' "$$" "$1"
`

func installationComponentWrite(root, path string, data []byte, mode os.FileMode) {
	GinkgoHelper()
	path = filepath.Join(root, filepath.FromSlash(path))
	Expect(os.MkdirAll(filepath.Dir(path), 0700)).To(Succeed())
	Expect(os.WriteFile(path, data, mode)).To(Succeed())
	Expect(os.Chmod(path, mode)).To(Succeed())
}

// Observe expected native identities directly, without using the production
// observation/hash/equality helpers as an oracle.
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:497
func installationComponentImage(root, path string, data []byte) *Image {
	GinkgoHelper()
	var stat unix.Stat_t
	Expect(unix.Lstat(filepath.Join(root, filepath.FromSlash(path)), &stat)).To(Succeed())
	id := &Identity{Device: uint64(stat.Dev), Inode: uint64(stat.Ino), User: stat.Uid, Group: stat.Gid, Mode: uint32(stat.Mode), Links: uint64(stat.Nlink), Size: stat.Size, ModifiedSeconds: stat.Mtim.Sec, ModifiedNanos: stat.Mtim.Nsec, ChangedSeconds: stat.Ctim.Sec, ChangedNanos: stat.Ctim.Nsec} // #nosec G115 -- Native device IDs are opaque identity bits; this signed-to-unsigned conversion performs no arithmetic.
	return &Image{Type: "regular", Mode: uint32(stat.Mode), SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), Bytes: int64(len(data)), Identity: id}
}

func installationComponentLayout(root string) (installedlayout.Descriptor, []byte, map[string]*Image) {
	GinkgoHelper()
	binary := []byte(preparationComponentChild)
	digest := fmt.Sprintf("%x", sha256.Sum256(binary))
	selection := []struct {
		path string
		data []byte
		mode os.FileMode
	}{
		{".factory/bin/factory-runtime", binary, 0700},
		{".factory/bin/runtime.manifest", []byte("independent inert component runtime control\n"), 0600},
		{".factory/bin/source.json", []byte("{\"kind\":\"component-fixture\"}\n"), 0600},
	}
	descriptor := installedlayout.Descriptor{SchemaVersion: 1, Kind: "go-hybrid-v1", Version: "v9.8.0", Revision: strings.Repeat("a", 40), Target: runtime.GOOS + "/" + runtime.GOARCH, RuntimeSHA256: digest, Assets: []installedlayout.Asset{}}
	observed := map[string]*Image{}
	for _, asset := range selection {
		installationComponentWrite(root, asset.path, asset.data, asset.mode)
		descriptor.Assets = append(descriptor.Assets, installedlayout.Asset{Path: asset.path, Mode: uint32(0100000 | asset.mode), SHA256: fmt.Sprintf("%x", sha256.Sum256(asset.data)), Bytes: int64(len(asset.data))})
		observed[asset.path] = installationComponentImage(root, asset.path, asset.data)
	}
	body, err := json.Marshal(descriptor)
	Expect(err).NotTo(HaveOccurred())
	installationComponentWrite(root, ".factory/installation.current", body, 0600)
	installationComponentWrite(root, ".factory-version", []byte("ref="+descriptor.Version+"\ncommit="+descriptor.Revision+"\n"), 0600)
	observed[".factory/installation.current"] = installationComponentImage(root, ".factory/installation.current", body)
	return descriptor, body, observed
}

func installationComponentOwner(root string) *transaction {
	GinkgoHelper()
	ctx := context.Background()
	owner := &transaction{root: root, published: map[string]*Image{}}
	var err error
	owner.lease, err = transition.RootExclusive(ctx, root)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() {
		if !owner.closed {
			Expect(owner.close(ctx)).To(Succeed())
		}
	})
	owner.guard, err = transition.Exclusive(ctx, root)
	Expect(err).NotTo(HaveOccurred())
	owner.tree, err = installationfs.Open(ctx, root)
	Expect(err).NotTo(HaveOccurred())
	return owner
}

var _ = Describe("Uncertain installation candidate preparation closure", func() {
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:483
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:489
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:150
	It("preserves actual candidate scratch and blocks ordinary admission after reported native exit uncertainty beside confirmed cleanup", func() {
		ctx := context.Background()
		base, err := filepath.EvalSymlinks(GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())
		for _, uncertain := range []bool{false, true} {
			name := "confirmed"
			if uncertain {
				name = "uncertain"
			}
			root, scratch := filepath.Join(base, name+"-root"), filepath.Join(base, name+"-scratch")
			Expect(os.Mkdir(root, 0700)).To(Succeed())
			Expect(os.Mkdir(scratch, 0700)).To(Succeed())
			candidate := filepath.Join(scratch, "candidate")
			_, _, images := installationComponentLayout(candidate)
			owner := installationComponentOwner(root)
			owner.scratch, owner.candidate = scratch, candidate
			owner.scratchIdentity, err = os.Lstat(scratch)
			Expect(err).NotTo(HaveOccurred())
			owner.request = Request{MigrationID: name + "-component", Profile: "bash-baseline", Confirmation: strings.Repeat("a", 64)}
			owner.record = journal{SchemaVersion: 1, Kind: "installation-transaction", MigrationID: owner.request.MigrationID, Purpose: "upgrade", Direction: "forward", Phase: "prepared", Profile: owner.request.Profile, Target: runtime.GOOS + "/" + runtime.GOARCH, PlanDigest: owner.request.Confirmation, Entries: []journalEntry{{Path: ".factory/bin/factory-runtime", Action: "create", Phase: "prepared", ExpectedAfter: images[".factory/bin/factory-runtime"]}}, Checks: []checkRecord{}, Outcome: "pending", Pending: nil}
			var actualPID int
			if uncertain {
				owner.operations = commandOps{executeObserved: func(ctx context.Context, root string, argv []string, environment map[string]string, allowance time.Duration, onSpawn func(context.Context, int) error) (native.CommandResult, error) {
					result, err := native.ExecuteCommandObserved(ctx, root, argv, environment, allowance, onSpawn)
					Expect(err).NotTo(HaveOccurred(), "real native child must first execute and complete")
					Expect(result.ExitConfirmed).To(BeTrue())
					Expect(result.OwnershipUnconfirmed).To(BeFalse())
					Expect(result.Stdout).To(ContainSubstring(fmt.Sprintf("CHILD_PID=%d ARG=--help", result.ProcessPID)))
					actualPID = result.ProcessPID
					Expect(syscall.Kill(-actualPID, 0)).To(Equal(syscall.ESRCH), "fixture cleanup must physically finish before reporting conservative observation uncertainty")
					result.OwnershipUnconfirmed, result.Outcome = true, "ownership_unconfirmed"
					return result, errors.Join(err, &native.OwnershipError{ProcessPID: actualPID}, unix.EIO)
				}}
			}
			runErr := owner.preflightCandidate(ctx, false)
			if uncertain {
				Expect(errors.Is(runErr, unix.EIO)).To(BeTrue())
				var ownership *native.OwnershipError
				Expect(errors.As(runErr, &ownership)).To(BeTrue())
				Expect(ownership.ProcessPID).To(Equal(actualPID))
			} else {
				Expect(runErr).NotTo(HaveOccurred())
			}
			launches, err := os.ReadFile(filepath.Join(candidate, ".factory/bin/factory-runtime.executions"))
			Expect(err).NotTo(HaveOccurred())
			journalBytes, err := os.ReadFile(filepath.Join(root, ".factory/installation-transactions", owner.request.MigrationID+".json"))
			Expect(err).NotTo(HaveOccurred())
			var persisted struct {
				Checks []struct {
					PID                  *int `json:"pid"`
					OwnershipUnconfirmed bool `json:"ownership_unconfirmed"`
				} `json:"checks"`
			}
			Expect(json.Unmarshal(journalBytes, &persisted)).To(Succeed())
			lines := strings.Split(strings.TrimSuffix(string(launches), "\n"), "\n")
			expectedLaunches := 2
			if uncertain {
				expectedLaunches = 1
			}
			Expect(lines).To(HaveLen(expectedLaunches))
			Expect(persisted.Checks).To(HaveLen(len(owner.record.Checks)))
			for index, line := range lines {
				var childPID int
				var argument string
				_, err := fmt.Sscanf(line, "%d\t%s", &childPID, &argument)
				Expect(err).NotTo(HaveOccurred())
				Expect(childPID).To(BeNumerically(">", 0))
				Expect(persisted.Checks[index].PID).NotTo(BeNil(), "actual onSpawn must durably identify launched child")
				Expect(*persisted.Checks[index].PID).To(Equal(childPID), "independent child handshake differs from saved native PID")
				Expect(persisted.Checks[index].OwnershipUnconfirmed).To(Equal(uncertain))
				Expect(syscall.Kill(-childPID, 0)).To(Equal(syscall.ESRCH))
			}
			closeErr := owner.close(ctx)
			current, scratchErr := os.Lstat(scratch)
			reader, readErr := transition.ReadOnly(ctx, root)
			if reader != nil {
				Expect(reader.Close(ctx)).To(Succeed())
			}
			ordinary, admissionErr := transition.Shared(ctx, root)
			if ordinary != nil {
				Expect(ordinary.Close(ctx, true)).To(Succeed())
			}
			_, _ = fmt.Fprintf(GinkgoWriter, "candidate closure %s: launched=%q; run=%v; close=%v; scratch=%v; reader=%v; ordinary=%v\n", name, launches, runErr, closeErr, scratchErr, readErr, admissionErr)
			if uncertain {
				Expect(errors.Is(errors.Join(runErr, closeErr), unix.EIO)).To(BeTrue())
				Expect(scratchErr).NotTo(HaveOccurred(), "unconfirmed child ownership cannot authorize scratch deletion")
				Expect(os.SameFile(owner.scratchIdentity, current)).To(BeTrue())
				binary, err := os.ReadFile(filepath.Join(candidate, ".factory/bin/factory-runtime"))
				Expect(err).NotTo(HaveOccurred())
				Expect(string(binary)).To(Equal(preparationComponentChild))
				Expect(readErr).To(HaveOccurred(), "releasing exclusion must retain durable evidence blocking pure ordinary readers")
				Expect(admissionErr).To(HaveOccurred(), "releasing exclusion must retain durable evidence blocking child/mutation work")
			} else {
				Expect(closeErr).NotTo(HaveOccurred())
				Expect(os.IsNotExist(scratchErr)).To(BeTrue(), "confirmed candidate preparation preserves existing cleanup behavior")
				Expect(readErr).NotTo(HaveOccurred())
				Expect(admissionErr).NotTo(HaveOccurred())
			}
		}
	})
})
