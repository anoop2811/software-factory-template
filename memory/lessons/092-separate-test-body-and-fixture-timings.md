# Separate test-body time from fixture setup

Ginkgo's per-spec `RunTime` can include a shared `BeforeAll` on the first
selected criterion. For focused fixture comparisons, inspect the `Node (End)`
event for `NodeType == It` and report setup and suite times separately.
Use the same harness and child-binary race policy on both sides; a focused
local pair does not establish hosted workflow savings.

Provenance: observed 2026-10-19 by reading the baseline and candidate Ginkgo
JSON from the two-criterion installation-image race runs. The first baseline
spec recorded 13.194497333 seconds including setup, while its It node recorded
10.389031375 seconds. The artifact directory was
`/var/folders/83/yj7qqyt551xbbpvm54tqkcbw0000gn/T/factory-fixture-compression-ci-policy-wipmvcn8`.
The canonical measurement contract is
`docs/adr/0105-installation-fixture-compression.md:39`; child race selection is
`acceptance/installation_image_test.go:234`.
