# Published RC4 adopter pilot

Contract: docs/DECISION_LOG.md:8951, docs/DECISION_LOG.md:8986 and
specs/001-go-runtime-conversion.md:486 (Q5).
Observed 2026-11-17 UTC on macOS 26.6.2, arm64.

**At RC4, R6 remained open.** The actual published candidate passed supported forward
migration, installed application checks and witnessed interrupted forward
recovery. Failed reverse preparation exposed a rollback/recovery defect, and a
customized ignore file exposed incomplete private-control ignore coverage.
R5 qualification and source corrections alone could not close these findings.
See the subsequent [signed stable pilot](GO_RELEASE_READINESS.md#signed-adopter-pilot-2026-11-20-utc).

## Scope

The operator-selected Go/TypeScript application has separate backend/frontend
directories and an older customized factory installation. Its actual checkout
was left unchanged. An independent exact copy was previewed first; a separate
fixture used the genuine immutable v0.1.6 initializer at
`b71ecc32e07ecd87eb330ba8e497c86612f92acd` around identical application source.

The supported fixture combines unchanged product Makefile targets with the
genuine v0.1.6 factory targets, the operator's configuration and a local pilot
decision log. It excludes the original unsupported custom lifecycle integration.
This demonstrates the supported migration path, not migration of the actual
older customized installation. The latter remains a deliberate refusal.

Candidate: `v0.2.0-rc.4`, source
`c51e0ae520839076c7245f2de7e396e4c3e546fe`. The installer was read from that
exact commit. The published raw bootstrap, runtime archive and installation
archive each passed real `gh attestation verify`, with independently obtained
trusted roots, the offline bundle, official repository/signer workflow, exact
source digest and tag, denial of self-hosted signers and exact subject/hash
matching. All three verification queries returned `true`, exit 0.

| Published darwin/arm64 subject | SHA-256 |
| --- | --- |
| `factory-bootstrap-v0.2.0-rc.4-darwin-arm64` | `6931a575d85ac1b17fe1be2d7535ab07e7e0b11810d0dba9da6563e994c507f2` |
| `factory-runtime-v0.2.0-rc.4-darwin-arm64.tar.gz` | `3adfd82733a44ea0015f3942ca802cdb82c01d69da8b45d6df06a811038f4553` |
| `factory-installation-v0.2.0-rc.4-darwin-arm64.tar.gz` | `3d70eee11c5b87bd25b26c53c1589bdc907abb0cf6151ed1a4d76e3276c51498` |

## Pilot evidence

| Observation | Status |
| --- | --- |
| Actual installation preview | Safe refusal, exit 2: 27 ownership/workflow blockers; all 212 copied files unchanged |
| Genuine v0.1.6 initialization and product baseline | Initializer exit 0: `159 passed, 0 failed, 8 skipped`; configured product checks passed |
| Published supported upgrade | Exit 0: `factory candidate: installation completed`; 103 application files and operator `factory.yaml` unchanged |
| Legacy recovery preservation | One ignored set, 50 regular non-executable files; old copied CI jobs retired and installed job created |
| Normal installed project checks | `./factory check`, exit 0: `PRE-PUSH: ALL CHECKS PASSED` |
| Doctor | Exit 0: 14 shipped gate proofs passed; two expected environment warnings below |
| Codex/Claude/OpenCode structural boundaries | All three `eval structural` commands passed; no claim of live client execution |
| Clean-checkout runtime/application checks | Authenticated `ci prepare` and installed checks passed locally with application-specific dependency setup; full hosted application CI not run |
| Interrupted forward installation | Observed partial publication, stopped/killed actual owner, admission refusal, then successful public forward recovery |
| Failed completed rollback | **Defect:** failed reverse health strands the committed transaction journal; Decision 176 correction required |
| Customized ignore policy | **Defect:** `.factory/runtime-transition.lock` remains addable; separate bounded correction required |
| Temporary feature failure/success/reversal cycle | Pending; deferred while correcting the pilot findings |
| Original checkout preservation | All 212 tracked files/modes, HEAD, index hash and status matched the initial snapshot |
| Disposable service cleanup | Owned database container, dedicated VM and data removed; Docker context remains `default` |
| Operator acceptance of the evidence and exit criteria | Pending |

## Commands and scope of observations

The paths below denote separately authenticated local release inputs and the
private disposable project. Actual argv, exit statuses, durations, stdout/stderr,
snapshots and transaction journals are retained in local evidence.

```sh
sh "$installer" upgrade --go-candidate v0.2.0-rc.4 --dry-run --json
sh "$installer" upgrade --go-candidate v0.2.0-rc.4 --confirm "$digest" --quiescent
./factory check
./factory doctor
./factory eval structural --harness=codex
./factory eval structural --harness=claude
./factory eval structural --harness=opencode
```

The supported upgrade took 28.759 seconds. Installed `factory check` took
49.690 seconds. Its configured application checks included Go lint and race
tests, frontend lint/types, 12 passing Vitest tests and a production frontend
build. The application's declared product command skips browser end-to-end
tests; this pilot does not claim those ran. Doctor reported the intentionally
unconfigured review secret and the clone's unselected tracked pre-push hook.
It did not silently change hook selection or enable paid review.

In a new independent clone of the committed upgraded installation:

```sh
sh "$installer" ci prepare --go-candidate v0.2.0-rc.4 \
  --origin-target darwin/arm64 --ephemeral
npm --prefix web ci --no-audit --no-fund
./factory selftest
./factory check
./factory eval structural --harness=opencode
./factory eval structural --harness=claude
./factory eval structural --harness=codex
```

Preparation returned `factory ci: prepared v0.2.0-rc.4 at
c51e0ae520839076c7245f2de7e396e4c3e546fe for darwin/arm64`, exit 0,
without changing tracked files. The subsequent installed-check sequence exited
0 in 61.771 seconds. This was local macOS execution, not a hosted Ubuntu job.

The generated workflow's literal root-level `npm ci` failed with `EUSAGE` because
the application lockfile lives in `web/`. The genuine v0.1.6 pack workflow has
the same root-level assumption. Nested Go/Node directories and the database
service need application-specific workflow setup; see
[CI preparation](CI_PREPARATION.md). No claim is made that the unmodified generated
workflow, remote security scanners or mutation jobs passed for this application.

## Recovery observations

The interruption observer started the authenticated published bootstrap directly.
It retained the unreaped child identity, observed journal phase `applying` with
multiple applied rows including the native runtime and a pending barrier,
sent SIGSTOP, confirmed that the actual owned process was stopped, then killed
its owned group and joined it. Exit evidence confirms SIGKILL. The stopped
journal was retained. Direct runtime selftest then refused with exit 2:
`factory: installed custody or admission unavailable`.

Using the same authenticated image, bundle, roots and profile, the public
`upgrade --installation --recover ID --direction forward` preview and confirmed
apply succeeded. The journal became `committed`, all four checks completed,
and installed `./factory selftest` exited 0. No journal or pending marker was
edited to obtain that outcome.

Completed rollback was separately attempted with legacy Python deliberately
unavailable. Refusal of legacy health was appropriate; subsequent state was not:

```text
factory upgrade: fresh legacy runtime health requires available baseline tools and readable current state
factory upgrade: completed rollback requires a committed forward installation
factory upgrade: incomplete installation pending marker is missing
```

The first diagnostic came from confirmed rollback; the second from a fresh
rollback preview with legacy tools restored; the third from reverse recovery
preview. The earlier committed journal was now `checking`, without a new pending
marker. Active installation files had not been reversed. The failed fixture is
preserved for diagnosis; no hand-written marker or journal restoration was used.
Decision 176 requires crash-safe preparation and independent public-CLI evidence
before claiming a correction. Published RC4 assets remain immutable.

## Evidence hygiene and remaining work

Initial fixture setup errors are retained in the private logs: the pilot needed
its own Git root, the supported factory Makefile rather than an uncopied custom
lifecycle hook, and the configured decision log/base ref. These were corrected
in the disposable fixture before the final supported upgrade and installed
checks. They are not reported as candidate regressions.

The actual older installation is not supported merely because a separately
initialized fixture passed. No application credentials were copied, no model
was called, and no private application source was published. The disposable
database used its own container/VM with temporary data and no host mounts.

[PR #187](https://github.com/anoop2811/software-factory-template/pull/187) merged
the rollback-preparation correction. Decision 177 covers the other finding.
On the retained RC4 checkout, `git ls-files --others --exclude-standard -- .factory/runtime-transition.lock`
listed the lock after upgrade. The corrected checkout should produce empty
output for that command, retain the lock on disk and leave unrelated files
addable under the adopter's rules.

## Rerun with both fixes in merged source

On 2026-11-20 from 14:38 to 14:45 UTC, the macOS arm64 pilot ran on a new
disposable copy of the supported v0.1.6 fixture. Its source was the
[merge of #188](https://github.com/anoop2811/software-factory-template/commit/fec964135850c0889c19c23fe4ab4e9a3a4ef1cb).
Its immediate parent was the [rollback fix from #187](https://github.com/anoop2811/software-factory-template/commit/a137eff0d8943c396698cffeb0a3162c012ad4b1).
The public RC4 assets still represent their original revision.

| Operation | Observed result |
| --- | --- |
| `factory upgrade --local --source DIR --dry-run --json`, followed by confirmed apply | v0.1.6 profile, zero blockers and conflicts, exit 0. All 103 application files retained their bytes and modes. The upgrade preserved the configuration, HEAD, index bytes and staged entries. Installed `./factory selftest` reported `14 shipped gates proved, 0 failed`. |
| Private lock visibility | The permanent lock existed at mode 0600. Of the lock and a neighboring operator file, Git listed only the operator file as addable. |
| Temporary feature through `./factory check` | Baseline passed with 12 frontend tests. A new test for an empty HTTP 205 response failed on JSON parsing. The implementation passed all 13 tests. Restoring the two original files returned the count to 12 and the full check passed again. All 103 application files, configuration, HEAD and staged entries matched the baseline. The raw Git index bytes changed during the feature exercise. |
| Rollback with the legacy Python prerequisite deliberately unavailable | Confirmed rollback refused with exit 2. The tool probe was reached. The 293 selected and tracked path observations retained their content, mode, link target or absence. The journal was in `checking` with a pending marker. |
| Recovery and fresh rollback after restoring Python | Public `upgrade --installation --recover ID --direction forward` preview and confirmed apply both exited 0. A fresh `--rollback ID` preview and confirmed apply exited 0. The journal recorded a committed reverse rollback. The 195 original tracked entries outside `.gitignore` matched, the permanent lock kept its inode, and the active Go binary, descriptor and pending marker were absent. The restored legacy `./factory metrics --json` exited 0. |

The 103-file comparison uses the application manifest. The 293-path comparison
takes the union of the legacy tracked paths, paths in the forward journal and
the two active Go runtime paths. The 195-entry comparison checks restoration
against the original 196 tracked entries, excluding the declared `.gitignore`
edit. The last two counts therefore measure different rollback steps.

`git diff --cached --name-only -z` returned no staged changes after feature
reversal. The subsequent rollback preserved the raw index. Comparing
`git ls-files --stage -z` to `git ls-tree -rz HEAD` then matched all 196 paths,
object IDs and modes, each at stage zero. This comparison establishes what a
commit would contain independently of the index file's byte representation.

This rerun exercised committed local-source delivery. The subsequent
[signed v0.2.0 pilot](GO_RELEASE_READINESS.md#signed-adopter-pilot-2026-11-20-utc)
exercised the release containing both fixes. The operator approved stable
publication after the merged pilot, as recorded in Decision 178.
