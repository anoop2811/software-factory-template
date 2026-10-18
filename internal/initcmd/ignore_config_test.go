package initcmd

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/installation"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Detached initialization ignore proof", func() {
	// per docs/adr/0101-go-release-handoff.md:517
	// per docs/adr/0101-go-release-handoff.md:525
	It("rejects canceled private rules independently of global Git exclusions", func() {
		root, err := os.MkdirTemp("", "factory-ignore-config-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(os.RemoveAll(root)).To(Succeed()) })
		clean, configured := filepath.Join(root, "clean"), filepath.Join(root, "configured")
		for _, home := range []string{clean, configured} {
			Expect(os.Mkdir(home, 0700)).To(Succeed())
		}
		excludes := filepath.Join(root, "external-excludes")
		Expect(os.WriteFile(excludes, []byte("/.factory/bin/factory-runtime\n"), 0600)).To(Succeed())
		configuration := "[core]\n excludesFile = " + strconv.Quote(excludes) + "\n"
		Expect(os.WriteFile(filepath.Join(configured, ".gitconfig"), []byte(configuration), 0600)).To(Succeed())
		complete := []byte(strings.Join(installation.PrivateIgnoreRules(), "\n") + "\n")
		canceled := append(append([]byte(nil), complete...), []byte("!/.factory/bin/\n")...)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		for _, home := range []string{clean, configured} {
			environment := map[string]string{}
			for _, entry := range os.Environ() {
				key, value, _ := strings.Cut(entry, "=")
				environment[key] = value
			}
			environment["HOME"], environment["XDG_CONFIG_HOME"] = home, filepath.Join(home, ".config")
			Expect(proveIgnoreRules(ctx, complete, environment)).To(Succeed(), "complete supplied rules must pass with HOME=%s", home)
			err := proveIgnoreRules(ctx, canceled, environment)
			Expect(err).To(HaveOccurred(), "canceled supplied rules must fail even when global config ignores the runtime: HOME=%s", home)
			Expect(err.Error()).To(ContainSubstring("effectively ignored"))
		}
		unchanged, err := os.ReadFile(filepath.Join(configured, ".gitconfig"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(unchanged)).To(Equal(configuration), "the proof must not repair an adopter's Git configuration")
	})
})
