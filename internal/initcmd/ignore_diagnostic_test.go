package initcmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/installation"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Detached init Git failure evidence", func() {
	// per docs/adr/0102-parallel-go-qualification.md:155
	// per docs/adr/0102-parallel-go-qualification.md:160
	DescribeTable("reports real refused Git results without disclosing private inputs or output", func(stage string, timeout bool) {
		root := GinkgoT().TempDir()
		tools, records, target := filepath.Join(root, "tools"), filepath.Join(root, "records"), filepath.Join(root, "target")
		for _, path := range []string{tools, records, target} {
			Expect(os.Mkdir(path, 0700)).To(Succeed())
		}
		script := `#!/bin/sh
printf '%s\n' "$$" >> "$DIAGNOSTIC_RECORDS/pids"
pwd -P > "$DIAGNOSTIC_RECORDS/cwd"
if [ "$DIAGNOSTIC_STAGE" = git-check-ignore ] && [ "$1" = ls-files ]; then exit 0; fi
printf PRIVATE_STDOUT
printf PRIVATE_STDERR >&2
if [ "$DIAGNOSTIC_TIMEOUT" = true ]; then exec /bin/sleep 30; fi
exit 7
`
		Expect(os.WriteFile(filepath.Join(tools, "git"), []byte(script), 0700)).To(Succeed()) // #nosec G306 -- private test-owned executable fixture requires its owner execute bit.
		environment := map[string]string{"PATH": tools + string(os.PathListSeparator) + os.Getenv("PATH"), "DIAGNOSTIC_RECORDS": records, "DIAGNOSTIC_STAGE": stage, "DIAGNOSTIC_TIMEOUT": strconv.FormatBool(timeout), "PRIVATE_ENV": "PRIVATE_ENVIRONMENT_VALUE"}
		rules := []byte(strings.Join(installation.PrivateIgnoreRules(), "\n") + "\n")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		prefix := "cannot prepare isolated ignore proof"
		var err error
		switch stage {
		case "git-init":
			err = proveIgnoreRules(ctx, rules, environment)
		case "git-boundary":
			prefix = "installation Git boundary is unavailable"
			Expect(os.WriteFile(filepath.Join(target, ".gitignore"), rules, 0600)).To(Succeed())
			opened, openErr := openTree(target)
			Expect(openErr).NotTo(HaveOccurred())
			DeferCleanup(opened.root.Close)
			err = checkPrivateIgnore(ctx, opened, environment)
		default:
			prefix = "private installation data must be untracked and effectively ignored"
			err = checkIgnoredPaths(ctx, target, environment)
		}
		Expect(ctx.Err()).NotTo(HaveOccurred(), "the existing command cap must decide before the outer test watchdog")
		Expect(err).To(HaveOccurred())
		pidData, readErr := os.ReadFile(filepath.Join(records, "pids"))
		Expect(readErr).NotTo(HaveOccurred())
		pids := strings.Fields(string(pidData))
		Expect(pids).NotTo(BeEmpty())
		for _, value := range pids {
			pid, parseErr := strconv.Atoi(value)
			Expect(parseErr).NotTo(HaveOccurred())
			Expect(syscall.Kill(pid, 0)).To(Equal(syscall.ESRCH))
			Expect(syscall.Kill(-pid, 0)).To(Equal(syscall.ESRCH))
		}
		cwdData, readErr := os.ReadFile(filepath.Join(records, "cwd"))
		Expect(readErr).NotTo(HaveOccurred())
		cwd := strings.TrimSpace(string(cwdData))
		if stage == "git-init" {
			_, statErr := os.Lstat(cwd)
			Expect(os.IsNotExist(statErr)).To(BeTrue(), "a confirmed closed isolated proof must leave no scratch directory")
		}
		message := err.Error()
		Expect(message).To(ContainSubstring(prefix))
		for _, private := range []string{"PRIVATE_STDOUT", "PRIVATE_STDERR", "PRIVATE_ENVIRONMENT_VALUE", root, cwd} {
			Expect(message).NotTo(ContainSubstring(private))
		}
		outcome, exit := "failed", 7
		if timeout {
			outcome, exit = "timeout", -int(syscall.SIGKILL)
		}
		field := func(name string) string {
			GinkgoHelper()
			match := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `=([^\s)]+)`).FindStringSubmatch(message)
			Expect(match).To(HaveLen(2), "safe result evidence must include %s", name)
			return match[1]
		}
		Expect(field("stage")).To(Equal(stage))
		Expect(field("outcome")).To(Equal(outcome))
		Expect(field("exit")).To(Equal(fmt.Sprint(exit)))
		Expect(field("pid")).To(Equal(pids[len(pids)-1]))
		Expect(field("exit_confirmed")).To(Equal("true"))
		Expect(field("ownership_unconfirmed")).To(Equal("false"))
		elapsed, parseErr := strconv.ParseFloat(field("elapsed_seconds"), 64)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(elapsed).To(BeNumerically(">=", 0))
		if timeout {
			Expect(elapsed).To(BeNumerically(">=", 5), "the real unchanged five-second cap must actually expire")
		}
	}, Entry("isolated preparation nonzero exit", "git-init", false), Entry("isolated preparation existing five-second timeout", "git-init", true), Entry("worktree-boundary nonzero exit", "git-boundary", false), Entry("tracked-path query nonzero exit", "git-ls-files", false), Entry("effective-ignore query nonzero exit", "git-check-ignore", false))
})
