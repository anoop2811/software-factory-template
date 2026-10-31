package acceptance_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/anoop2811/software-factory-template/internal/transition"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Go default packaged upgrade", Ordered, ContinueOnFailure, func() {
	var fixtures *defaultUpgradeJourneyFixtures
	BeforeAll(func() { fixtures = prepareDefaultUpgradeJourneys(true) })

	// per docs/adr/0122-default-packaged-go-upgrade.md:23
	It("honors explicit and ambient stable refs while allowing equal releases without retention advance", func() {
		p := fixtures.legacyProject(defaultUpgradeNextTag)
		snapshot := installationSnapshot(p.root)
		p.f.environment["FACTORY_REF"] = defaultUpgradeNextTag
		proposal := p.preview("--")
		Expect(proposal["checks"]).To(ContainElement(defaultUpgradePlanMarker))
		Expect(handoffRecorded(p.f, "api")).To(ContainElement("repos/anoop2811/software-factory-template/commits/refs/tags/" + defaultUpgradeNextTag))
		p.f.environment["FACTORY_REF"] = "main"
		proposal = p.preview("--ref=" + defaultUpgradeTag)
		Expect(proposal["blockers"]).To(BeEmpty())
		Expect(handoffRecorded(p.f, "api")).To(ContainElement("repos/anoop2811/software-factory-template/commits/refs/tags/" + defaultUpgradeTag))
		Expect(installationSnapshot(p.root)).To(Equal(snapshot))
		defaultInitNoForbidden(p.f)
		equal := fixtures.goProject(defaultUpgradeTag)
		before := installationSnapshot(equal.root)
		same := equal.preview()
		Expect(same["blockers"]).To(BeEmpty())
		// per docs/adr/0106-go-backup-retention.md:22
		Expect(retentionPreviewReport(same).Advanced).To(BeFalse())
		Expect(installationSnapshot(equal.root)).To(Equal(before))
		applied := equal.apply(same)
		Expect(applied.status).To(Equal(0), "equal supported release remains eligible: %+v", applied)
		report := retentionApplyReport(applied)
		Expect(report.Advanced).To(BeFalse(), "same-release transaction recovery does not authorize expiry")
		Expect(report.RemovedSets).To(BeZero())
		defaultInitDescriptor(equal.root, defaultUpgradeTag, fixtures.revision, equal.before.binary)
		defaultInitNoForbidden(equal.f)
	})

	// per docs/adr/0122-default-packaged-go-upgrade.md:60
	It("authenticates official predecessors before downgrade refusal and distinguishes unavailable history", func() {
		p := fixtures.goProject(defaultUpgradeNextTag)
		snapshot := installationSnapshot(p.root)
		refused := p.run("--dry-run", "--json")
		Expect(refused.status).To(Equal(2), "ordinary default must not implicitly reuse the newer installed version: %+v", refused)
		Expect(refused.stdout).To(BeEmpty())
		Expect(refused.stderr).To(ContainSubstring("refusing a downgrade"))
		Expect(handoffRecorded(p.f, "api")).To(ContainElement("repos/anoop2811/software-factory-template/commits/refs/tags/" + defaultUpgradeNextTag))
		Expect(handoffFlag(handoffRecorded(p.f, "verify"), "--source-ref")).To(Equal("refs/tags/" + defaultUpgradeNextTag))
		Expect(installationSnapshot(p.root)).To(Equal(snapshot))
		explicit := p.run("--go-candidate", defaultUpgradeTag, "--dry-run", "--json")
		Expect(explicit.status).To(Equal(0), "explicit candidate retains its existing downgrade policy: %+v", explicit)
		Expect(installationSnapshot(p.root)).To(Equal(snapshot))
		historicalAsset := filepath.Join(p.f.environment["HANDOFF_ASSETS"], "factory-installation-"+defaultUpgradeNextTag+"-"+p.f.target+".tar.gz")
		Expect(os.Remove(historicalAsset)).To(Succeed())
		unavailable := p.run("--dry-run", "--json")
		Expect(unavailable.status).To(Equal(1), "a valid but unavailable official predecessor is acquisition failure, not invented unsupported policy: %+v", unavailable)
		Expect(unavailable.stderr).To(ContainSubstring("predecessor acquisition failed"))
		Expect(installationSnapshot(p.root)).To(Equal(snapshot))
		Expect(retentionBackupIDs(p.root)).To(BeEmpty())
		defaultInitNoForbidden(p.f)
	})

	// per docs/adr/0122-default-packaged-go-upgrade.md:52
	It("refuses unsupported predecessor hints without inventing release acquisition or changing bytes", func() {
		p := fixtures.goProject(defaultUpgradeTag)
		path := filepath.Join(p.root, ".factory/installation.current")
		original, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		for _, label := range []string{"local-" + fixtures.revision, "not-a-release", "v0.2", "v0.1.6"} {
			var descriptor map[string]any
			Expect(json.Unmarshal(original, &descriptor)).To(Succeed())
			descriptor["version"] = label
			writeFixture(path, imageMarshal(descriptor), 0600)
			snapshot := installationSnapshot(p.root)
			out := p.run("--dry-run", "--json")
			Expect(out.status).To(Equal(2), "untrusted hint %q: %+v", label, out)
			Expect(out.stdout).To(BeEmpty())
			Expect(out.stderr).To(ContainSubstring("--local"))
			Expect(out.stderr).To(ContainSubstring("--before-source"))
			Expect(out.stderr).To(ContainSubstring("--installation"))
			Expect(handoffRecorded(p.f, "api")).To(ContainElement("repos/anoop2811/software-factory-template/commits/refs/tags/" + defaultUpgradeTag))
			Expect(installationSnapshot(p.root)).To(Equal(snapshot))
			defaultInitNoForbidden(p.f)
		}
		writeFixture(path, original, 0600)
	})

	// per docs/adr/0122-default-packaged-go-upgrade.md:31
	// per docs/adr/0122-default-packaged-go-upgrade.md:44
	It("validates strict selectors before acquisition while preserving explicit owners and source assessment", func() {
		f, driver := fixtures.surface(defaultUpgradeTag)
		snapshot := installationSnapshot(f.cwd)
		f.environment["FACTORY_REF"] = "main"
		for _, args := range [][]string{{"upgrade", "--help"}, {"upgrade", "-h"}, {"upgrade", "--go-candidate", defaultUpgradeBeforeRC, "--help"}, {"upgrade", "--local", "--help"}, {"upgrade", "--installation", "--help"}, {"upgrade", "--keep-backup", "operator-held", "--help"}} {
			out := handoffNativeProcess(f.cwd, f.environment, "", driver, args...)
			Expect(out.status).To(Equal(0), "help owner %q: %+v", args, out)
			Expect(out.stderr).To(BeEmpty())
			defaultUpgradeNoAcquisition(f)
		}
		invalid := [][]string{{"--ref"}, {"--ref="}, {"--ref", "main"}, {"--ref", "v0.2"}, {"--ref", defaultUpgradeBeforeRC}, {"--ref", "v0.1.6"}, {"--ref", defaultUpgradeTag, "--ref", defaultUpgradeTag}, {"--ref", defaultUpgradeTag, "--go-candidate", defaultUpgradeTag}, {"--source", fixtures.source}, {"--unknown"}, {"--", "--ref", defaultUpgradeTag}}
		for _, args := range invalid {
			for _, help := range []bool{false, true} {
				flags := append([]string{"upgrade"}, args...)
				if help {
					flags = append(flags, "--help")
				}
				out := handoffNativeProcess(f.cwd, f.environment, "", driver, flags...)
				Expect(out.status).To(Equal(2), "strict %q: %+v", flags, out)
				defaultUpgradeNoAcquisition(f)
			}
		}
		delete(f.environment, "FACTORY_REF")
		imageGit(f.cwd, "init", "--quiet")
		caller := installationSnapshot(f.cwd)
		assessed := handoffNativeProcess(f.cwd, f.environment, "", driver, "upgrade", "--dry-run", "--source", fixtures.source, "--json")
		Expect(assessed.status).To(Equal(2), "legacy read-only source assessment retains partial/unproven semantics: %+v", assessed)
		var assessment map[string]any
		Expect(json.Unmarshal([]byte(assessed.stdout), &assessment)).To(Succeed())
		Expect(assessment["mode"]).To(Equal("dry_run"))
		Expect(assessment["coverage"]).To(Equal("partial"))
		Expect(assessment["ownership_basis"]).To(Equal("unproven"))
		Expect(installationSnapshot(f.cwd)).To(Equal(caller))
		defaultUpgradeNoAcquisition(f)
		for path, value := range snapshot {
			Expect(installationSnapshot(f.cwd)[path]).To(Equal(value))
		}
	})

	// per docs/adr/0122-default-packaged-go-upgrade.md:77
	It("preserves omitted cache and user history while blocking customization of a consumed required asset", func() {
		p := fixtures.goProject(defaultUpgradeTag)
		omitted := ".factory/assets/scaffold/scripts/lib/loop.py"
		consumed := ".factory/assets/scaffold/templates/metrics.html"
		writeFixture(filepath.Join(p.root, omitted), []byte("operator retained former cache\n"), 0600)
		writeFixture(filepath.Join(p.root, "docs/upgrade-user-note.md"), []byte("preserve application history\n"), 0600)
		before := installationSnapshot(p.root)
		proposal := p.preview()
		Expect(proposal["blockers"]).To(BeEmpty())
		Expect(installationSnapshot(p.root)).To(Equal(before))
		writeFixture(filepath.Join(p.root, consumed), []byte("operator customized required metrics template\n"), 0600)
		customized := installationSnapshot(p.root)
		blocked := p.run("--dry-run", "--json")
		Expect(blocked.status).To(Equal(2), "required active cache customization remains a blocker: %+v", blocked)
		Expect(blocked.stdout).To(ContainSubstring(consumed))
		Expect(installationSnapshot(p.root)).To(Equal(customized))
		refused := p.apply(proposal)
		Expect(refused.status).To(Equal(2), "stale approval cannot overwrite customization: %+v", refused)
		Expect(installationSnapshot(p.root)).To(Equal(customized))
		Expect(retentionBackupIDs(p.root)).To(BeEmpty())
		defaultInitNoForbidden(p.f)
	})

	// per docs/adr/0122-default-packaged-go-upgrade.md:9
	It("fails unavailable unauthenticated or nonstable selections without fallback or project mutation", func() {
		p := fixtures.legacyProject()
		snapshot := installationSnapshot(p.root)
		foreign := installationSnapshot(p.f.environment["FACTORY_HOME"])
		for _, scenario := range []string{"release-absent", "marked-prerelease", "authentication-fail", "wrong-archive-subject", "missing-image"} {
			p.f.environment["HANDOFF_SCENARIO"] = scenario
			out := p.run("--dry-run", "--json")
			Expect(out.status).To(Equal(1), "selection failure %s: %+v", scenario, out)
			Expect(out.stdout).To(BeEmpty())
			Expect(installationSnapshot(p.root)).To(Equal(snapshot))
			Expect(installationSnapshot(p.f.environment["FACTORY_HOME"])).To(Equal(foreign))
			Expect(handoffRecorded(p.f, "release-view")).To(ContainElement("tagName,isDraft,isPrerelease"))
			defaultInitNoForbidden(p.f)
		}
	})

	// per docs/adr/0122-default-packaged-go-upgrade.md:71
	It("uses installed exclusive admission and refuses pending publication without retaining its own reader", func() {
		p := fixtures.goProject(defaultUpgradeTag)
		fixtures.provide(p.f, defaultUpgradeNextTag)
		installed := filepath.Join(p.root, "factory")
		flags := p.args("--ref", defaultUpgradeNextTag, "--dry-run", "--json")
		proposal := handoffProposal(handoffNativeProcess(p.root, p.f.environment, "", installed, flags...))
		before := installationSnapshot(p.root)
		lease, err := transition.ReadOnly(context.Background(), p.root)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = lease.Close(context.Background()) })
		confirm := p.args("--ref", defaultUpgradeNextTag, "--confirm", proposal["proposal_digest"].(string), "--quiescent")
		refused := handoffNativeProcess(p.root, p.f.environment, "", installed, confirm...)
		Expect(refused.status).To(Equal(2), "another reader still blocks EX publication: %+v", refused)
		Expect(installationSnapshot(p.root)).To(Equal(before))
		Expect(lease.Close(context.Background())).To(Succeed())
		applied := handoffNativeProcess(p.root, p.f.environment, "", installed, confirm...)
		Expect(applied.status).To(Equal(0), "the installed public caller must release its own SH before target EX: %+v", applied)
		Expect(applied.stderr).To(ContainSubstring(defaultUpgradeApplyMarker))
		defaultInitDescriptor(p.root, defaultUpgradeNextTag, fixtures.revision, fixtures.image(defaultUpgradeNextTag).binary)
		pending := filepath.Join(p.root, ".factory/runtime-publication.pending")
		settled := installationSnapshot(p.root)
		_, err = os.Lstat(pending)
		Expect(os.IsNotExist(err)).To(BeTrue(), "successful publication must clear its own pending marker before the evaluator creates one")
		witness := []byte("operator pending publication witness\n")
		writeFixture(pending, witness, 0600)
		held := installationSnapshot(p.root)
		programs := []string{installed, filepath.Join(p.root, ".factory/bin/factory-runtime")}
		for _, program := range programs {
			blocked := handoffNativeProcess(p.root, p.f.environment, "", program, p.args("--ref", defaultUpgradeNextTag, "--dry-run", "--json")...)
			Expect(blocked.status).To(Equal(2), "pending publication %s: %+v", program, blocked)
			Expect(blocked.stderr).To(Equal("factory candidate: physical installation root lease unavailable\n"))
			Expect(installationSnapshot(p.root)).To(Equal(held))
		}
		// The RootError intentionally masks its operational cause. Prove the
		// actual marker is causal rather than requiring a more revealing label.
		// per docs/adr/0122-default-packaged-go-upgrade.md:71
		Expect(os.ReadFile(pending)).To(Equal(witness))
		Expect(os.Remove(pending)).To(Succeed())
		Expect(installationSnapshot(p.root)).To(Equal(settled))
		for _, program := range programs {
			unblocked := handoffNativeProcess(p.root, p.f.environment, "", program, p.args("--ref", defaultUpgradeNextTag, "--dry-run", "--json")...)
			Expect(unblocked.status).To(Equal(0), "removing only the owned witness restores the same read-only request %s: %+v", program, unblocked)
			Expect(handoffProposal(unblocked)["checks"]).To(ContainElement(defaultUpgradePlanMarker))
			Expect(installationSnapshot(p.root)).To(Equal(settled))
		}
		defaultInitNoForbidden(p.f)
	})
})
