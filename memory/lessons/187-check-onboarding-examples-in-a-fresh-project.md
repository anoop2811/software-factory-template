# Check plan examples from a freshly initialized project

A local plan can require project state even when it launches no model. The
v0.2.0 loop planner resolves Git HEAD in internal/loop/snapshot.go:53 and needs
a configured check as described in docs/LOOPS.md:38. A bare initialization can
lack both. Put these prerequisites next to examples meant for new adopters.

Observed 2026-11-20 during the homepage audit. In the fresh macOS arm64
installation, `./factory loop plan --harness codex --session demo --task first
--json` returned exit 2. In a disposable copy with a commit and configured
check, the same command returned a manual plan, and the manual run passed.
