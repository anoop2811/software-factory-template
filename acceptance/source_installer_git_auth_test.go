package acceptance_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const sourceAuthGitHandshake = "--factory-installer-git-auth-probe"

var sourceAuthGitTransportNames = []string{
	"GIT_ASKPASS", "GIT_TERMINAL_PROMPT", "GIT_SSH", "GIT_SSH_COMMAND", "GIT_SSH_VARIANT",
	"GIT_SSL_CAINFO", "GIT_SSL_CAPATH", "GIT_SSL_CERT", "GIT_SSL_KEY",
	"GIT_SSL_CERT_PASSWORD_PROTECTED", "GIT_SSL_CERT_TYPE", "GIT_SSL_KEY_TYPE",
	"GIT_SSL_VERSION", "GIT_SSL_CIPHER_LIST", "GIT_SSL_NO_VERIFY",
	"GIT_HTTP_PROXY_AUTHMETHOD", "GIT_PROXY_SSL_CAINFO", "GIT_PROXY_SSL_CERT",
	"GIT_PROXY_SSL_KEY", "GIT_PROXY_SSL_CERT_PASSWORD_PROTECTED", "GIT_PROXY_COMMAND",
}

var sourceAuthOtherNames = []string{"SSH_AUTH_SOCK", "HTTPS_PROXY", "http_proxy", "NO_PROXY"}

type sourceAuthGitCall struct {
	Args  []string
	Git   map[string]string
	Other map[string]string
}

// This evaluator-only Git records synthetic inputs and never accesses a network.
// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:265
func init() {
	if len(os.Args) < 2 || os.Args[1] != sourceAuthGitHandshake {
		return
	}
	call := sourceAuthGitCall{Args: os.Args[2:], Git: map[string]string{}, Other: map[string]string{}}
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "GIT_") {
			call.Git[key] = value
		}
	}
	for _, name := range sourceAuthOtherNames {
		if value, present := os.LookupEnv(name); present {
			call.Other[name] = value
		}
	}
	body, err := json.Marshal(call)
	if err != nil {
		os.Exit(97)
	}
	file, err := os.CreateTemp(os.Getenv("SOURCE_AUTH_TRACE"), "git-*.json") // #nosec G703 -- private evaluator handshake and test-owned trace directory; synthetic values only.
	if err != nil {
		os.Exit(97)
	}
	if _, err := file.Write(body); err != nil {
		os.Exit(97)
	}
	if err := file.Close(); err != nil {
		os.Exit(97)
	}
	args := call.Args
	if len(args) < 10 || args[0] != "-C" ||
		strings.Join(args[2:9], "\x00") != strings.Join([]string{"--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "-c", "core.hooksPath=/dev/null"}, "\x00") {
		os.Exit(98)
	}
	switch strings.Join(args[9:], "\x00") {
	case "remote\x00get-url\x00origin":
		fmt.Println(os.Getenv("SOURCE_AUTH_ORIGIN"))
	case "status\x00--porcelain\x00--untracked-files=all":
	case "fetch\x00--quiet\x00--depth\x001\x00origin\x00feature/auth":
	case "ls-remote\x00--exit-code\x00--heads\x00origin\x00feature/auth":
	case "checkout\x00--quiet\x00-B\x00feature/auth\x00FETCH_HEAD":
	case "rev-parse\x00--show-toplevel":
		fmt.Println(os.Getenv("SOURCE_AUTH_CALLER"))
	case "rev-parse\x00--is-shallow-repository":
		fmt.Println("false")
	default:
		os.Exit(98)
	}
	os.Exit(0)
}

func sourceAuthEnvironment() map[string]string {
	values := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		values[key] = value
	}
	return values
}

// The existing branch criterion owns this additional transport boundary.
// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:265
func sourceInstallerGitAuth() {
	GinkgoHelper()
	Expect(sourceAuthGitTransportNames).To(HaveLen(21))
	parentBefore := sourceAuthEnvironment()
	failures := []string{}
	for _, variant := range []string{"literal", "empty"} {
		root, caller := fixtureDirectories()
		base := filepath.Dir(root)
		cache, trace, tools := filepath.Join(base, "auth cache"), filepath.Join(base, "Git observations"), filepath.Join(base, "auth tools")
		Expect(os.MkdirAll(filepath.Join(cache, ".git"), 0700)).To(Succeed())
		Expect(os.Mkdir(trace, 0700)).To(Succeed())
		upgrade, err := os.ReadFile("../scripts/factory-upgrade.sh")
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(cache, "scripts/factory-upgrade.sh"), upgrade, 0755)
		writeFixture(filepath.Join(cache, "factory"), []byte(sourceEntrypointRecorder), 0755)
		writeFixture(filepath.Join(caller, "factory.yaml"), []byte("project_name: synthetic-auth\n"), 0600)
		installer, err := os.ReadFile("../install.sh")
		Expect(err).NotTo(HaveOccurred())
		installerPath := filepath.Join(base, "actual installer.sh")
		writeFixture(installerPath, installer, 0755)
		writeFixture(filepath.Join(tools, "git"), []byte("#!/bin/sh\nexec \"$SOURCE_AUTH_PROBE\" "+sourceAuthGitHandshake+" \"$@\"\n"), 0755)
		record, input := filepath.Join(base, "adapter argv"), filepath.Join(base, "adapter input")
		origin := "https://example.invalid/synthetic-template.git"
		environment := map[string]string{
			"PATH":         tools + string(os.PathListSeparator) + "/usr/bin:/bin",
			"FACTORY_HOME": cache, "FACTORY_REPO": origin, "SOURCE_AUTH_TRACE": trace,
			"SOURCE_AUTH_PROBE": sourceCompilerHelperImage, "SOURCE_AUTH_ORIGIN": origin, "SOURCE_AUTH_CALLER": caller,
			"SOURCE_ENTRY_ARGV": record, "SOURCE_ENTRY_INPUT": input, "SOURCE_ENTRY_LITERAL_ENV": "auth literal",
			"GORACE": "atexit_sleep_ms=0", "BASH_ENV": "", "ENV": "",
		}
		expected := map[string]string{}
		for index, name := range sourceAuthGitTransportNames {
			value := ""
			if variant == "literal" {
				value = fmt.Sprintf("synthetic[%d] ' $(not-code)\nsecond line\tend", index)
			}
			expected[name], environment[name] = value, value
		}
		for _, name := range []string{
			"GIT_DIR", "GIT_COMMON_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
			"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_CONFIG", "GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_0",
			"GIT_CONFIG_VALUE_0", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM", "GIT_CONFIG_NOSYSTEM",
			"GIT_EXEC_PATH", "GIT_TEMPLATE_DIR", "GIT_NAMESPACE", "GIT_REPLACE_REF_BASE",
			"GIT_ALLOW_PROTOCOL", "GIT_PROTOCOL", "GIT_PROTOCOL_FROM_USER", "GIT_OPTIONAL_LOCKS",
			"GIT_TRACE", "GIT_TRACE_PACKET", "GIT_TRACE_CURL", "GIT_TRACE2", "GIT_TRACE2_EVENT",
			"GIT_ASKPASS_EXTRA", "GIT_SSH_COMMAND_EXTRA", "GIT_SSL_CAINFO_EXTRA", "GIT_PROXY_COMMAND_EXTRA",
		} {
			environment[name] = "synthetic redirection poison"
		}
		other := map[string]string{}
		for _, name := range sourceAuthOtherNames {
			other[name], environment[name] = "synthetic ordinary credential discovery", "synthetic ordinary credential discovery"
		}
		cacheBefore, callerBefore := installationSnapshot(cache), installationSnapshot(caller)
		result := handoffNativeProcess(caller, environment, "transport input\n", "/bin/sh", installerPath, "upgrade", "--ref", "feature/auth")
		Expect(installationSnapshot(cache)).To(Equal(cacheBefore))
		Expect(installationSnapshot(caller)).To(Equal(callerBefore))
		Expect(maps.Equal(parentBefore, sourceAuthEnvironment())).To(BeTrue(), "parent environment must remain unchanged; never print its potentially sensitive values")
		Expect(result.status).To(Equal(63), "actual installer must complete the owned refresh/handoff: %+v", result)
		raw, err := os.ReadFile(record)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")).To(Equal([]string{caller, "auth literal", "upgrade", "--local", "--source", cache}))
		Expect(os.ReadFile(input)).To(Equal([]byte("transport input\n")))
		files, err := os.ReadDir(trace)
		Expect(err).NotTo(HaveOccurred())
		Expect(files).To(HaveLen(8), "exercise five identity and three refresh invocations")
		identity, refresh, preserved := 0, 0, true
		for _, file := range files {
			body, err := os.ReadFile(filepath.Join(trace, file.Name()))
			Expect(err).NotTo(HaveOccurred())
			var call sourceAuthGitCall
			Expect(json.Unmarshal(body, &call)).To(Succeed())
			mode := call.Args[9]
			if mode == "fetch" || mode == "ls-remote" || mode == "checkout" {
				refresh++
				if !maps.Equal(call.Git, expected) {
					preserved = false
					failures = append(failures, variant+": refresh must preserve exactly all21 literal/empty transport values")
				}
			} else {
				identity++
				Expect(maps.Equal(call.Git, map[string]string{"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull})).To(BeTrue(), "identity must remove every inherited GIT_* key")
			}
			Expect(maps.Equal(call.Other, other)).To(BeTrue(), "ordinary credential/proxy discovery remains inherited")
		}
		Expect(identity).To(Equal(5))
		Expect(refresh).To(Equal(3))
		AddReportEntry("source-entrypoint.authenticated-refresh", map[string]any{"variant": variant, "transport_names": len(expected), "identity_calls": identity, "refresh_calls": refresh, "literal_values_preserved": preserved, "parent_environment_unchanged": maps.Equal(parentBefore, sourceAuthEnvironment()), "cache_and_project_preserved": true}, ReportEntryVisibilityNever)
	}
	Expect(failures).To(BeEmpty(), "transport-authentication discovery must survive refresh without granting repository redirection")
}
