# Include conditional initializer inputs in installed assets

Observed 2026-10-09 via the compiled installed `factory init --pack=none`
regression: an existing application README reached review, while an empty target
failed for a missing required template. The initializer conditionally reads
README.md; collecting only its ordinary scaffold copies missed that dependency.

Use the complete initializer plan, including conditional source inputs, when
reviewing an installed asset selection. Keep canonical template data separate
from the application's existing files. The contract and corrected selection live
in docs/adr/0098-whole-installation-upgrade-and-rollback.md:435; the conditional
read is internal/initcmd/plan.go:100 and its public regression is
acceptance/installed_runtime_test.go:226. This lesson points to that canon rather
than maintaining another installation inventory.
