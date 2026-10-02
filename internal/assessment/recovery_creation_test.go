package assessment

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

func creationGit(root string, args ...string) {
	GinkgoHelper()
	command := exec.Command("git", args...) // #nosec G204 -- literal Git fixture operations, no shell or external operands.
	command.Dir = root
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull}
	data, err := command.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "%s", data)
}

func creationIgnored(root, path string) bool {
	GinkgoHelper()
	command := exec.Command("git", "check-ignore", "-q", "--", path) // #nosec G204 -- fixed relative test paths in a fixture-local Git query.
	command.Dir = root
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull}
	err := command.Run()
	if err == nil {
		return true
	}
	var exitErr *exec.ExitError
	Expect(errors.As(err, &exitErr)).To(BeTrue())
	Expect(exitErr.ExitCode()).To(Equal(1))
	return false
}

func creationFixture() (string, string, RecoveryRequest, map[string]string) {
	GinkgoHelper()
	root, path := adoptionMatchingFixture()
	Expect(os.Chmod(root, 0700)).To(Succeed())
	creationGit(root, "init", "-q")
	creationGit(root, "add", "--", "scripts")
	Expect(os.WriteFile(filepath.Join(root, ".git/info/exclude"), []byte("# PRIVATE_EXISTING_EXCLUDE\n"), 0600)).To(Succeed())
	proposal, err := ProposeAdoption(context.Background(), root, []string{"scripts/factory-budget.sh"})
	Expect(err).NotTo(HaveOccurred())
	request := RecoveryRequest{MigrationID: "fault-set-1", TargetRevision: strings.Repeat("a", 40), Paths: []string{"scripts/factory-budget.sh"}, Confirmation: proposal.ProposalDigest}
	return root, path, request, map[string]string{"PATH": os.Getenv("PATH"), "HOME": root}
}

type creationDescriptors struct {
	files []*os.File
	names map[*os.File]string
}

func creationControls() (*creationDescriptors, recoveryWriteOps) {
	GinkgoHelper()
	descriptors := &creationDescriptors{names: map[*os.File]string{}}
	controls := recoveryWriteOps{open: func(_ context.Context, parent *os.File, name string, flags int, mode uint32) (*os.File, error) {
		if _, exists := descriptors.names[parent]; !exists {
			descriptors.files = append(descriptors.files, parent)
			descriptors.names[parent] = filepath.Base(parent.Name())
		}
		fd, err := unix.Openat(int(parent.Fd()), name, flags, mode)
		if err != nil {
			return nil, err
		}
		file := os.NewFile(uintptr(fd), name)
		descriptors.files = append(descriptors.files, file)
		descriptors.names[file] = name
		return file, nil
	}}
	return descriptors, controls
}

func creationNoCompletion(root, id string) {
	GinkgoHelper()
	_, err := os.Lstat(filepath.Join(root, ".factory/backups", id, "manifest.json"))
	Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue(), "incomplete recovery must not publish completion")
}

func creationFailure(result RecoveryCreation, err error, status int) {
	GinkgoHelper()
	Expect(err).To(HaveOccurred())
	Expect(ErrorStatus(err)).To(Equal(status))
	Expect(result.Result).NotTo(BeElementOf("created", "already_present"))
	Expect(err.Error()).NotTo(ContainSubstring("PRIVATE_"))
}

var _ = Describe("Durable recovery creation controlled I/O", func() {
	// per docs/adr/0091-durable-local-recovery-creation.md:164
	// per docs/adr/0091-durable-local-recovery-creation.md:169
	DescribeTable("keeps failed exclusion staging inert and activation confined to the complete narrow rule", func(stage string, active bool) {
		root, _, request, environment := creationFixture()
		Expect(os.Mkdir(filepath.Join(root, ".factory"), 0700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, ".factory/events.log"), []byte("PRIVATE_RUNTIME_KEEP"), 0600)).To(Succeed())
		excludePath := filepath.Join(root, ".git/info/exclude")
		before, err := os.ReadFile(excludePath)
		Expect(err).NotTo(HaveOccurred())
		identity, err := os.Stat(excludePath)
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		descriptors, controls := creationControls()
		staged, activated, triggered := false, false, false
		controls.write = func(_ context.Context, file *os.File, data []byte) (int, error) {
			n, err := file.Write(data)
			if descriptors.names[file] == "exclude" && n == len(data) && err == nil {
				staged = true
				if stage == "staging cancellation" {
					triggered = true
					cancel()
				}
			}
			return n, err
		}
		controls.writeAt = func(_ context.Context, file *os.File, data []byte, offset int64) (int, error) {
			if descriptors.names[file] != "exclude" {
				return file.WriteAt(data, offset)
			}
			Expect(staged).To(BeTrue())
			Expect(data).To(Equal([]byte{'/'}), "activate the entire staged rule with exactly one byte")
			if stage == "activation short write" {
				triggered = true
				return 0, nil
			}
			if stage == "activation write error" {
				triggered = true
				return 0, errors.New("PRIVATE_ACTIVATION_FAILURE")
			}
			n, err := file.WriteAt(data, offset)
			activated = n == len(data) && err == nil
			if stage == "activation cancellation" {
				triggered = true
				cancel()
			}
			return n, err
		}
		controls.syncFile = func(_ context.Context, file *os.File) error {
			if descriptors.names[file] == "exclude" && ((stage == "staged sync" && staged && !activated) || (stage == "activation sync" && activated)) {
				triggered = true
				return syscall.EIO
			}
			return file.Sync()
		}
		controls.read = func(_ context.Context, file *os.File, buffer []byte) (int, error) {
			n, err := file.Read(buffer)
			if descriptors.names[file] == "exclude" && !triggered && ((stage == "staged read error" && staged && !activated) || (stage == "staged read mismatch" && staged && !activated) || (stage == "activation read mismatch" && activated)) {
				triggered = true
				if stage == "staged read error" {
					return n, errors.New("PRIVATE_EXCLUDE_READBACK_FAILURE")
				}
				Expect(n).To(BeNumerically(">", 0))
				buffer[0] ^= 1
			}
			return n, err
		}
		result, err := createRecovery(ctx, root, request, environment, controls)
		Expect(triggered).To(BeTrue(), "fault boundary was not exercised")
		creationFailure(result, err, 1)
		Expect(RecoveryStateMayRemain(err)).To(BeTrue())
		if strings.Contains(stage, "cancellation") {
			Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		}
		creationNoCompletion(root, request.MigrationID)
		Expect(creationIgnored(root, ".factory/events.log")).To(BeFalse())
		Expect(creationIgnored(root, ".factory/backups/")).To(Equal(active))
		after, readErr := os.ReadFile(excludePath)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(after)).To(HavePrefix(string(before)))
		if active {
			Expect(string(after)).To(Equal(string(before) + "/.factory/backups/\n"))
		} else {
			Expect(string(after)).To(Equal(string(before) + "#.factory/backups/\n"))
		}
		afterIdentity, statErr := os.Stat(excludePath)
		Expect(statErr).NotTo(HaveOccurred())
		Expect(os.SameFile(identity, afterIdentity)).To(BeTrue())
		recoveryClosed(descriptors.files)
	}, Entry("cancellation after inert append", "staging cancellation", false), Entry("inert append sync failure", "staged sync", false), Entry("inert append readback error", "staged read error", false), Entry("inert append readback mismatch", "staged read mismatch", false), Entry("zero-byte activation", "activation short write", false), Entry("activation error", "activation write error", false), Entry("activation sync failure", "activation sync", true), Entry("activation readback mismatch", "activation read mismatch", true), Entry("cancellation after activation", "activation cancellation", true))

	// per docs/adr/0091-durable-local-recovery-creation.md:163
	// per docs/adr/0091-durable-local-recovery-creation.md:171
	It("keeps unrelated runtime paths visible after a partial local exclusion write", func() {
		root, _, request, environment := creationFixture()
		Expect(os.Mkdir(filepath.Join(root, ".factory"), 0700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, ".factory/events.log"), []byte("PRIVATE_RUNTIME_KEEP"), 0600)).To(Succeed())
		excludePath := filepath.Join(root, ".git/info/exclude")
		before, err := os.ReadFile(excludePath)
		Expect(err).NotTo(HaveOccurred())
		identity, err := os.Stat(excludePath)
		Expect(err).NotTo(HaveOccurred())
		Expect(creationIgnored(root, ".factory/events.log")).To(BeFalse())
		descriptors, controls := creationControls()
		triggered := false
		controls.write = func(_ context.Context, file *os.File, data []byte) (int, error) {
			if descriptors.names[file] == "exclude" {
				triggered = true
				return file.Write(data[:len(data)/2])
			}
			return file.Write(data)
		}
		result, err := createRecovery(context.Background(), root, request, environment, controls)
		Expect(triggered).To(BeTrue())
		creationFailure(result, err, 1)
		Expect(RecoveryStateMayRemain(err)).To(BeTrue())
		creationNoCompletion(root, request.MigrationID)
		Expect(creationIgnored(root, ".factory/events.log")).To(BeFalse(), "a failed partial append must not hide the whole runtime directory")
		after, readErr := os.ReadFile(excludePath)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(after)).To(HavePrefix(string(before)))
		afterIdentity, statErr := os.Stat(excludePath)
		Expect(statErr).NotTo(HaveOccurred())
		Expect(os.SameFile(identity, afterIdentity)).To(BeTrue())
		recoveryClosed(descriptors.files)
	})

	// per docs/adr/0091-durable-local-recovery-creation.md:76
	// per docs/adr/0091-durable-local-recovery-creation.md:96
	// per docs/adr/0083-go-recovery-set-inspection.md:89
	DescribeTable("rejects special bits in ordinary file and directory metadata", func(bit string) {
		file := unix.Stat_t{Mode: unix.S_IFREG | 0600, Uid: uint32(os.Geteuid()), Nlink: 1, Size: 1}
		directory := unix.Stat_t{Mode: unix.S_IFDIR | 0700, Uid: uint32(os.Geteuid())}
		Expect(ownedFile(file, 64<<10)).To(BeTrue())
		Expect(recoveryFile(file, 64<<10)).To(BeTrue())
		Expect(trustedDirectory(directory, true)).To(BeTrue())
		switch bit {
		case "setuid":
			file.Mode |= 04000
			directory.Mode |= 04000
		case "setgid":
			file.Mode |= 02000
			directory.Mode |= 02000
		case "sticky":
			file.Mode |= 01000
			directory.Mode |= 01000
		}
		Expect(ownedFile(file, 64<<10)).To(BeFalse())
		Expect(recoveryFile(file, 64<<10)).To(BeFalse())
		Expect(trustedDirectory(directory, false)).To(BeFalse())
	}, Entry("setuid metadata", "setuid"), Entry("setgid metadata", "setgid"), Entry("sticky metadata", "sticky"))

	// per docs/adr/0091-durable-local-recovery-creation.md:114
	// per docs/adr/0091-durable-local-recovery-creation.md:120
	// per docs/adr/0091-durable-local-recovery-creation.md:141
	DescribeTable("preserves failure evidence without publishing completion and closes descriptors", func(stage string) {
		root, original, request, environment := creationFixture()
		before, err := os.ReadFile(original)
		Expect(err).NotTo(HaveOccurred())
		descriptors, controls := creationControls()
		triggered := false
		controls.write = func(_ context.Context, file *os.File, data []byte) (int, error) {
			name := descriptors.names[file]
			if !triggered && ((stage == "exclude short write" && name == "exclude") || (stage == "payload short write" && name == "factory-budget.sh")) {
				triggered = true
				return file.Write(data[:len(data)/2])
			}
			if !triggered && stage == "payload write error" && name == "factory-budget.sh" {
				triggered = true
				n, writeErr := file.Write(data[:len(data)/2])
				if writeErr != nil {
					return n, writeErr
				}
				return n, errors.New("PRIVATE_WRITE_ERROR")
			}
			return file.Write(data)
		}
		controls.syncFile = func(_ context.Context, file *os.File) error {
			if (stage == "exclude sync" && descriptors.names[file] == "exclude") || (stage == "payload sync" && descriptors.names[file] == "factory-budget.sh") {
				triggered = true
				return errors.New("PRIVATE_SYNC_ERROR")
			}
			return file.Sync()
		}
		controls.syncDirectory = func(_ context.Context, file *os.File) error {
			name := descriptors.names[file]
			if (stage == "Git info sync" && name == "info") || (stage == "nested sync" && name == "scripts") || (stage == "files sync" && name == "files") {
				triggered = true
				return syscall.EIO
			}
			return file.Sync()
		}
		result, err := createRecovery(context.Background(), root, request, environment, controls)
		Expect(triggered).To(BeTrue(), "fault collaborator was never exercised")
		creationFailure(result, err, 1)
		creationNoCompletion(root, request.MigrationID)
		after, readErr := os.ReadFile(original)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
		recoveryClosed(descriptors.files)
	}, Entry("partial local exclusion", "exclude short write"), Entry("local exclusion sync", "exclude sync"), Entry("Git info directory sync", "Git info sync"), Entry("partial saved copy", "payload short write"), Entry("saved copy error after bytes", "payload write error"), Entry("saved copy sync", "payload sync"), Entry("nested payload directory sync", "nested sync"), Entry("files directory sync", "files sync"))

	// per docs/adr/0091-durable-local-recovery-creation.md:115
	// per docs/adr/0091-durable-local-recovery-creation.md:120
	DescribeTable("requires saved-file readback rather than trusting successful writes", func(corrupt bool) {
		root, _, request, environment := creationFixture()
		descriptors, controls := creationControls()
		triggered := false
		controls.read = func(_ context.Context, file *os.File, buffer []byte) (int, error) {
			n, err := file.Read(buffer)
			if !triggered && descriptors.names[file] == "factory-budget.sh" {
				stat, statErr := file.Stat()
				Expect(statErr).NotTo(HaveOccurred())
				if stat.Mode().Perm() == 0600 {
					triggered = true
					if corrupt && n > 0 {
						buffer[0] ^= 1
					} else if !corrupt {
						return n, errors.New("PRIVATE_READBACK_FAILURE")
					}
				}
			}
			return n, err
		}
		result, err := createRecovery(context.Background(), root, request, environment, controls)
		Expect(triggered).To(BeTrue())
		creationFailure(result, err, 1)
		creationNoCompletion(root, request.MigrationID)
		recoveryClosed(descriptors.files)
	}, Entry("readback I/O error", false), Entry("readback digest mismatch", true))

	// per docs/adr/0091-durable-local-recovery-creation.md:120
	DescribeTable("reports post-publication durability failure while retaining the manifest and copies", func(stage string) {
		root, _, request, environment := creationFixture()
		descriptors, controls := creationControls()
		published, triggered := false, false
		controls.write = func(_ context.Context, file *os.File, data []byte) (int, error) {
			n, err := file.Write(data)
			if descriptors.names[file] == "manifest.json" && n == len(data) && err == nil {
				published = true
			}
			return n, err
		}
		controls.syncFile = func(_ context.Context, file *os.File) error {
			if stage == "manifest" && descriptors.names[file] == "manifest.json" {
				triggered = true
				return syscall.EIO
			}
			return file.Sync()
		}
		controls.syncDirectory = func(_ context.Context, file *os.File) error {
			name := descriptors.names[file]
			if published && ((stage == "set" && name == request.MigrationID) || (stage == "backups" && name == "backups") || (stage == "factory" && name == ".factory") || (stage == "root" && file == descriptors.files[0])) {
				triggered = true
				return syscall.EIO
			}
			return file.Sync()
		}
		result, err := createRecovery(context.Background(), root, request, environment, controls)
		Expect(published).To(BeTrue())
		Expect(triggered).To(BeTrue())
		creationFailure(result, err, 1)
		data, readErr := os.ReadFile(filepath.Join(root, ".factory/backups", request.MigrationID, "manifest.json"))
		Expect(readErr).NotTo(HaveOccurred())
		_, valid := parseRecoveryManifest(context.Background(), data, request.MigrationID)
		Expect(valid).To(BeTrue(), "durability failure must preserve parseable evidence")
		recoveryClosed(descriptors.files)
	}, Entry("manifest file", "manifest"), Entry("set directory", "set"), Entry("backup root", "backups"), Entry("factory directory", "factory"), Entry("installation directory", "root"))

	// per docs/adr/0091-durable-local-recovery-creation.md:120
	// per docs/adr/0091-durable-local-recovery-creation.md:130
	It("leaves an interrupted reserved set visible and refuses to overwrite it on retry", func() {
		root, _, request, environment := creationFixture()
		descriptors, controls := creationControls()
		actualOpen := controls.open
		triggered := false
		controls.open = func(ctx context.Context, parent *os.File, name string, flags int, mode uint32) (*os.File, error) {
			if name == "factory-budget.sh" && flags&unix.O_CREAT != 0 {
				triggered = true
				return nil, syscall.ENOSPC
			}
			return actualOpen(ctx, parent, name, flags, mode)
		}
		result, err := createRecovery(context.Background(), root, request, environment, controls)
		Expect(triggered).To(BeTrue())
		creationFailure(result, err, 1)
		creationNoCompletion(root, request.MigrationID)
		inventory, inspectErr := InspectRecovery(context.Background(), root)
		Expect(inspectErr).NotTo(HaveOccurred())
		Expect(inventory.Sets).To(HaveLen(1))
		Expect(inventory.Sets[0].Classification).To(Equal("unrecognized"))
		Expect(inventory.Sets[0].FileCount).To(BeZero())
		Expect(inventory.Sets[0].Bytes).To(BeZero())
		info, statErr := os.Stat(filepath.Join(root, ".factory/backups", request.MigrationID))
		Expect(statErr).NotTo(HaveOccurred())
		result, err = CreateRecovery(context.Background(), root, request, environment)
		creationFailure(result, err, 2)
		after, statErr := os.Stat(filepath.Join(root, ".factory/backups", request.MigrationID))
		Expect(statErr).NotTo(HaveOccurred())
		Expect(os.SameFile(info, after)).To(BeTrue())
		creationNoCompletion(root, request.MigrationID)
		recoveryClosed(descriptors.files)
	})
})

var _ = Describe("Durable recovery creation cancellation and identity", func() {
	// per docs/adr/0091-durable-local-recovery-creation.md:57
	// per docs/adr/0091-durable-local-recovery-creation.md:120
	DescribeTable("propagates cancellation at a real operation boundary and closes descriptors", func(stage string) {
		root, _, request, environment := creationFixture()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		descriptors, controls := creationControls()
		actualOpen := controls.open
		triggered := false
		controls.open = func(ctx context.Context, parent *os.File, name string, flags int, mode uint32) (*os.File, error) {
			file, err := actualOpen(ctx, parent, name, flags, mode)
			if stage == "payload open" && name == "factory-budget.sh" && flags&unix.O_CREAT != 0 {
				triggered = true
				cancel()
			}
			return file, err
		}
		controls.write = func(_ context.Context, file *os.File, data []byte) (int, error) {
			n, err := file.Write(data)
			if stage == "payload write" && descriptors.names[file] == "factory-budget.sh" {
				triggered = true
				cancel()
			}
			return n, err
		}
		controls.syncDirectory = func(_ context.Context, file *os.File) error {
			err := file.Sync()
			if stage == "directory sync" && descriptors.names[file] == "files" {
				triggered = true
				cancel()
			}
			return err
		}
		result, err := createRecovery(ctx, root, request, environment, controls)
		Expect(triggered).To(BeTrue())
		creationFailure(result, err, 1)
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		creationNoCompletion(root, request.MigrationID)
		recoveryClosed(descriptors.files)
	}, Entry("saved-file open", "payload open"), Entry("saved-file write", "payload write"), Entry("directory sync", "directory sync"))

	// per docs/adr/0091-durable-local-recovery-creation.md:43
	// per docs/adr/0091-durable-local-recovery-creation.md:96
	// per docs/adr/0091-durable-local-recovery-creation.md:115
	DescribeTable("refuses changes to selected leaves and pinned ancestors before completion", func(kind string) {
		root, original, request, environment := creationFixture()
		descriptors, controls := creationControls()
		triggered, evidenceRoot := false, root
		controls.syncFile = func(_ context.Context, file *os.File) error {
			if !triggered && descriptors.names[file] == "factory-budget.sh" {
				triggered = true
				switch kind {
				case "source bytes":
					Expect(os.WriteFile(original, []byte("PRIVATE_CHANGED_ORIGINAL"), 0600)).To(Succeed())
				case "source mode":
					Expect(os.Chmod(original, 0700)).To(Succeed())
				case "source inode":
					data, err := os.ReadFile(original)
					Expect(err).NotTo(HaveOccurred())
					Expect(os.Rename(original, original+".held")).To(Succeed())
					Expect(os.WriteFile(original, data, 0600)).To(Succeed())
					Expect(os.Chmod(original, 0755)).To(Succeed())
				case "source ancestor":
					target := filepath.Join(root, "scripts")
					Expect(os.Rename(target, target+".held")).To(Succeed())
					Expect(os.Mkdir(target, 0700)).To(Succeed())
					Expect(os.WriteFile(original, []byte("PRIVATE_REPLACEMENT"), 0600)).To(Succeed())
					Expect(os.Chmod(original, 0755)).To(Succeed())
				case "backup ancestor":
					target := filepath.Join(root, ".factory/backups")
					Expect(os.Rename(target, target+".held")).To(Succeed())
					Expect(os.Mkdir(target, 0700)).To(Succeed())
				case "installation":
					evidenceRoot = root + "-held"
					Expect(os.Rename(root, evidenceRoot)).To(Succeed())
					DeferCleanup(os.RemoveAll, evidenceRoot)
					Expect(os.Mkdir(root, 0700)).To(Succeed())
				case "installation mode":
					Expect(os.Chmod(root, 0770)).To(Succeed())
				}
			}
			return file.Sync()
		}
		result, err := createRecovery(context.Background(), root, request, environment, controls)
		Expect(triggered).To(BeTrue())
		creationFailure(result, err, 2)
		creationNoCompletion(root, request.MigrationID)
		if kind == "backup ancestor" {
			_, statErr := os.Lstat(filepath.Join(root, ".factory/backups.held", request.MigrationID, "manifest.json"))
			Expect(errors.Is(statErr, os.ErrNotExist)).To(BeTrue())
		} else {
			creationNoCompletion(evidenceRoot, request.MigrationID)
		}
		recoveryClosed(descriptors.files)
	}, Entry("selected source content", "source bytes"), Entry("selected source mode", "source mode"), Entry("same-byte source inode", "source inode"), Entry("selected source ancestor", "source ancestor"), Entry("recovery ancestor", "backup ancestor"), Entry("installation root", "installation"), Entry("installation mode", "installation mode"))
})

var _ = Describe("Durable recovery creation reservation and reuse", func() {
	// per docs/adr/0091-durable-local-recovery-creation.md:105
	// per docs/adr/0091-durable-local-recovery-creation.md:107
	DescribeTable("serializes concurrent creators without overwriting reservations or exceeding capacity", func(capacity bool) {
		root, _, request, environment := creationFixture()
		if capacity {
			backups := filepath.Join(root, ".factory/backups")
			Expect(os.MkdirAll(backups, 0700)).To(Succeed())
			for i := range 63 {
				Expect(os.Mkdir(filepath.Join(backups, "occupied-"+strings.Repeat("a", i+1)), 0700)).To(Succeed())
			}
		}
		started, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		descriptors, controls := creationControls()
		controls.write = func(_ context.Context, file *os.File, data []byte) (int, error) {
			if descriptors.names[file] == "factory-budget.sh" {
				once.Do(func() { close(started) })
				<-release
			}
			return file.Write(data)
		}
		type outcome struct {
			result RecoveryCreation
			err    error
		}
		completed := make(chan outcome, 1)
		go func() {
			defer GinkgoRecover()
			result, err := createRecovery(context.Background(), root, request, environment, controls)
			completed <- outcome{result, err}
		}()
		DeferCleanup(func() {
			once.Do(func() { close(started) })
			select {
			case <-release:
			default:
				close(release)
			}
		})
		Eventually(started, 5*time.Second).Should(BeClosed())
		other := request
		if capacity {
			other.MigrationID = "other-reservation"
		}
		second, secondErr := CreateRecovery(context.Background(), root, other, environment)
		creationFailure(second, secondErr, 2)
		creationNoCompletion(root, request.MigrationID)
		close(release)
		var first outcome
		Eventually(completed, 5*time.Second).Should(Receive(&first))
		Expect(first.err).NotTo(HaveOccurred())
		Expect(first.result.Result).To(Equal("created"))
		entries, err := os.ReadDir(filepath.Join(root, ".factory/backups"))
		Expect(err).NotTo(HaveOccurred())
		if capacity {
			Expect(entries).To(HaveLen(64))
			second, secondErr = CreateRecovery(context.Background(), root, other, environment)
			creationFailure(second, secondErr, 2)
			after, readErr := os.ReadDir(filepath.Join(root, ".factory/backups"))
			Expect(readErr).NotTo(HaveOccurred())
			Expect(after).To(HaveLen(64))
		} else {
			Expect(entries).To(HaveLen(1))
		}
		recoveryClosed(descriptors.files)
	}, Entry("same-ID concurrent reservation", false), Entry("concurrent capacity boundary", true))

	// per docs/adr/0091-durable-local-recovery-creation.md:124
	DescribeTable("re-syncs an intact existing set and rejects failed reuse without changing its bytes", func(directory bool) {
		root, _, request, environment := creationFixture()
		created, err := CreateRecovery(context.Background(), root, request, environment)
		Expect(err).NotTo(HaveOccurred())
		Expect(created.Result).To(Equal("created"))
		manifestPath := filepath.Join(root, ".factory/backups", request.MigrationID, "manifest.json")
		before, err := os.ReadFile(manifestPath)
		Expect(err).NotTo(HaveOccurred())
		descriptors, controls := creationControls()
		triggered := false
		controls.syncFile = func(_ context.Context, file *os.File) error {
			if !directory && descriptors.names[file] == "factory-budget.sh" {
				triggered = true
				return syscall.EIO
			}
			return file.Sync()
		}
		controls.syncDirectory = func(_ context.Context, file *os.File) error {
			if directory && descriptors.names[file] == request.MigrationID {
				triggered = true
				return syscall.EIO
			}
			return file.Sync()
		}
		result, err := createRecovery(context.Background(), root, request, environment, controls)
		Expect(triggered).To(BeTrue())
		creationFailure(result, err, 1)
		after, readErr := os.ReadFile(manifestPath)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
		recoveryClosed(descriptors.files)
	}, Entry("saved file resync", false), Entry("set directory resync", true))
})
