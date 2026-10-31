package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var initComponentOnce sync.Once
var initComponentProgram []byte

// These historical engine criteria retain real process, inherited input, TTY and
// cancellation boundaries. They no longer claim public default delivery, which
// has a separate authenticated outside-in family. This executable is test-owned
// and has no production flag, ambient trust variable or installed role.
// per docs/adr/0118-default-packaged-go-initialization.md:66
func nativeInitComponent() []byte {
	GinkgoHelper()
	initComponentOnce.Do(func() {
		root, err := filepath.Abs("..")
		Expect(err).NotTo(HaveOccurred())
		parent := filepath.Join(root, ".factory")
		Expect(os.MkdirAll(parent, 0700)).To(Succeed())
		work, err := os.MkdirTemp(parent, "test-init-component-")
		Expect(err).NotTo(HaveOccurred())
		defer os.RemoveAll(work)
		source := filepath.Join(work, "main.go")
		writeFixture(source, []byte(`package main
import("context";"os";"path/filepath";"github.com/anoop2811/software-factory-template/internal/configuredcmd";"github.com/anoop2811/software-factory-template/internal/initcmd")
func main(){
 executable,err:=os.Executable();if err!=nil||filepath.Base(executable)!="factory-init-component"||len(os.Args)<2||os.Args[1]!="init"{os.Exit(2)}
 source,err:=filepath.Abs(filepath.Dir(os.Args[0]));if err!=nil{os.Exit(2)}
 os.Exit(initcmd.Run(context.Background(),os.Args[2:],source,configuredcmd.CaptureEnvironment(),os.Stdin,os.Stdout,os.Stderr))
}
`), 0600)
		output := filepath.Join(work, "factory-init-component")
		args := []string{"build", "-trimpath", "-buildvcs=false", "-o", output, source}
		if os.Getenv("FACTORY_CLI_TEST_RACE") == "1" {
			args = append([]string{"build", "-race"}, args[1:]...)
		}
		command := exec.Command("go", args...) // #nosec G204 G702 -- evaluator-owned source/output and fixed compiler arguments, inside the existing module.
		command.Dir = root
		data, err := command.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "test-only initializer driver: %s", data)
		initComponentProgram, err = os.ReadFile(output)
		Expect(err).NotTo(HaveOccurred())
	})
	return initComponentProgram
}
