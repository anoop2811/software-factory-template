package main

import (
	"context"
	"fmt"
	"github.com/anoop2811/software-factory-template/internal/installation"
	"github.com/anoop2811/software-factory-template/internal/installationcmd"
	"github.com/anoop2811/software-factory-template/internal/installedlayout"
	"github.com/anoop2811/software-factory-template/internal/releasecmd"
	"os"

	"github.com/anoop2811/software-factory-template/internal/bridge"
	"github.com/anoop2811/software-factory-template/internal/cli"
	"github.com/anoop2811/software-factory-template/internal/configuredcmd"
)

func main() { os.Exit(run(context.Background(), os.Args[1:])) }

// Installed admission precedes both public and private sourceable protocols.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:275.
func run(ctx context.Context, args []string) (status int) {
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "factory: executable unavailable")
		return 1
	}
	root, physical, err := installedlayout.PhysicalRoot(ctx, executable)
	if err != nil {
		fmt.Fprintln(os.Stderr, "factory: installed physical root unavailable")
		return 1
	}
	var expected []string
	if len(args) > 0 && args[0] == "--installed" {
		if !physical || len(args) < 5 || args[4] != "--" {
			fmt.Fprintln(os.Stderr, "factory: invalid installed invocation")
			return 2
		}
		expected = args[1:4]
		args = args[5:]
	}
	if physical {
		// Whole routes acquire EX directly; never upgrade a retained shared lease.
		if len(args) > 1 && args[0] == "upgrade" && (installationcmd.Claimed(args[1:]) || releasecmd.Claimed(args[1:]) || releasecmd.BackupClaimed(args[1:]) || releasecmd.LocalUpgradeClaimed(args[1:])) && os.Getenv("FACTORY_BRIDGE_PROTOCOL") == "" {
			// Mark the physical caller without retaining SH; the transaction owns EX.
			// docs/adr/0117-explicit-go-local-upgrade.md:59.
			return cli.RunInstalled(ctx, args, root, "")
		}
		selected, err := installedlayout.Open(ctx, root, expected)
		if err != nil {
			fmt.Fprintln(os.Stderr, "factory: installed custody or admission unavailable")
			return installation.ErrorStatus(err)
		}
		defer func() {
			if err := selected.Close(context.WithoutCancel(ctx)); err != nil {
				fmt.Fprintln(os.Stderr, "factory: installed admission closure failed")
				status = 1
			}
		}()
		if os.Getenv("FACTORY_BRIDGE_PROTOCOL") != "" {
			return bridge.Run(ctx, args)
		}
		return cli.RunInstalled(ctx, args, selected.Root, selected.Assets)
	}
	if len(expected) != 0 {
		fmt.Fprintln(os.Stderr, "factory: installed marker has no physical installation")
		return 2
	}

	// Candidate sourceable calls use an explicit private protocol; ordinary
	// invocation retains its public dispatcher. docs/DECISION_LOG.md:1960.
	if os.Getenv("FACTORY_BRIDGE_PROTOCOL") != "" {
		return bridge.Run(ctx, args)
	}
	// Installed custody and sourceable protocols retain their existing owners.
	// docs/adr/0116-explicit-go-local-initialization.md:64.
	if releasecmd.LocalImageClaimed(args) {
		return releasecmd.RunLocalImage(ctx, args, configuredcmd.CaptureEnvironment(), os.Stdin, os.Stdout, os.Stderr)
	}
	if releasecmd.LocalUpgradeImageClaimed(args) {
		return releasecmd.RunLocalUpgradeImage(ctx, args, configuredcmd.CaptureEnvironment(), os.Stdout, os.Stderr)
	}
	return cli.Run(ctx, args)
}
