# kubeloop — handoff

_Snapshot: 2026-07-08. Live at github.com/Karthikbangari/kubeloop- (`main`, 24 commits, all pushed)._

## What it is
A read-only Kubernetes rightsizing CLI (single Go binary, `github.com/kubeloop/kubeloop`).
It scans workloads, ranks wasted CPU/memory requests **in dollars**, and prepares the fix
as a **reviewed pull request** — it never writes to a cluster. Pitch: *"KRR tells you the
right numbers; kubeloop ranks the waste and gets it merged."* See `README.md` and
`plan/MASTER-PLAN-LOOPED.md`.

## Status: v0.1 offline is complete and working
Everything that can run without a live cluster is built, tested, and hardened.

- `kubeloop scan --from-file <json>` → dollar-ranked waste table (confidence, JVM cautions,
  exclusions with reasons, under-provisioned flagged separately, immediate-vs-node-consolidation
  labeling), text or `--json`. Pricing overridable via `--pricing-file`.
- `kubeloop scan --from-manifests <dir> --usage-file <json>` → the same report read straight from
  real Kubernetes manifests (a GitOps repo) plus a Prometheus usage export, with no cluster access.
  Sources are mutually exclusive; unknown fields in the usage export are hard errors; a workload
  with no usage entry is excluded with a reason, never sized on no data.
- `kubeloop pr --from-file <json> --manifest <yaml> --workload X --container app --out <path>`
  → locates the source manifest, patches only the reduced requests, writes it, and prints a PR
  title/body (evidence, rollback, read-only disclaimer). Refuses multi-container pods, ambiguous
  names, no-op/under-provisioned targets.
- `kubeloop --version`, `make ci` green, `deploy/rbac.yaml` (least-privilege read-only), CI +
  GoReleaser + Homebrew-less packaging.

## Quickstart
```bash
export PATH="$HOME/.local/go/bin:$PATH"   # Go 1.23 was installed here
make ci                                   # go vet + go test ./...  (all green)
make build                                # -> bin/kubeloop
./bin/kubeloop scan --from-file examples/offline-input.json
./bin/kubeloop scan --from-manifests examples/manifests --usage-file examples/manifests-usage.json
```
Note: local test runs use a workspace-local cache: `GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache`
(the default `~/Library/Caches` path is sandbox-blocked here). CI uses defaults and is fine.

## Architecture
Full map in `docs/architecture.md`. Layering (leaves → composition):
- `internal/rightsizing` — domain types, recommender, **safety floors** (trust core, pure leaf).
- `internal/{labels,savings}` — leaves (collision labels; bill-realization wording).
- `internal/safety` — exclusions (batch, <7d history, **missing metrics signal**), confidence, JVM caution.
- `internal/reporting` — pricing, dollar-ranking, single table renderer.
- `internal/classify` — waste vs under-provisioned vs right-sized.
- `internal/scan` — orchestrator → `Report`.
- `internal/pr` (+ `/quantity`) — locate → patch → compose → prepare, with guards.
- `internal/inventory`, `internal/readlayer` — read-layer. `readlayer.ToScanInput` is the **one**
  place a workload becomes scan input; every read source funnels through it. Subpackages:
  `/promusage` + `/promclient` (Prometheus), `/quantityparse` → `/kubeparse` → `/manifestsource`
  → `/dirsource` (manifests → `[]scan.Input`, backs `--from-manifests`).
- `cmd/kubeloop` — the CLI (`scan`/`pr`/`--version`).

## Done since the last snapshot
- ✅ **Read-layer graduated** (RULEBOOK #71, Codex-approved): the 5 playground slices moved into
  `internal/readlayer/{promclient,quantityparse,kubeparse,manifestsource,dirsource}`, and
  `--from-manifests` shipped. `playground/` is now empty. Two cleanups landed with it: the
  duplicated `scan.Input` assembly collapsed into `readlayer.ToScanInput`, and dirsource's
  double-parse (`ponytail:` debt) folded away via `manifestsource.FromWorkload`.

## What's left (2 code milestones, both gated by environment — not by lack of work)
1. **Live cluster reader** — real kube API `LIST` + validated PromQL. **Needs a cluster +
   Prometheus** (and a `client-go` dependency decision; today the project is yaml.v3-only).
   Most of the path already exists: `kubeparse` parses what a `LIST` returns, `promclient`
   issues the query, `readlayer.ToScanInput` assembles the result. What's missing is the
   client wrapper and the PromQL *strings* (they need a real Prometheus to validate).
2. **Real GitHub PR creation** — git branch/commit + GitHub API; Helm/Kustomize source mapping.
   **Needs a token + target repo + those tools.**
After that it's a live v1.0. Plan "loops" B/C/D (launch, revenue, expansion) are business, not code.

## Development workflow (important — this is how the repo has been built)
Two agents, gated by `RULEBOOK.md`:
1. **Claude Code** builds each change in `playground/slice-NN-*/` first, logs it in `RULEBOOK.md`.
2. **Codex** reviews the playground slice and records a verdict in the log.
3. Only after ✅ does the code **graduate** into `internal/`, and the playground slice is removed.
`RULEBOOK.md` is the source of truth: 71 numbered log entries, newest first. Read the top few to
see current state and any Codex notes. Bug-fixes to already-graduated code are made directly and
reviewed after. `playground/` is currently empty — the next slice starts there.

Caveat worth knowing: entries #64–#70 are still `⬜ awaiting review`. They are already-graduated
bug-fixes and polish (fail-loud decoders, pluralization, `--version`, the no-op-PR guard), so the
code is live but has not had a second pair of eyes. Worth a Codex pass.

## Known limitations (see docs/architecture.md "Known limitations")
- Patcher: single-document, 2-space raw-YAML manifests; verifies identity; preserves limits/comments/quote-style.
  Multi-doc and Helm/Kustomize source mapping are the tool-backed tail.
- `pr` reduce-only compares the scan's current vs proposal, not the manifest's — identical in the
  real (live) flow, divergent only in offline `--from-file`.
- PromQL query *strings* are deliberately not written yet (need a live Prometheus to validate);
  `promclient` takes the query as input for exactly that reason.

## Git / deploy
- Remote `origin` = `https://github.com/Karthikbangari/kubeloop-.git`, branch `main`, in sync.
- Push auth: use a GitHub **Personal Access Token** (repo scope) as the password, or in the URL:
  `git push https://<PAT>@github.com/Karthikbangari/kubeloop-.git main`. (The old cached
  `antonysoumya` credential was the earlier 403; it's cleared.)
- Normal pushes from here (no `--force` needed — histories are aligned).
- A release is cut by tagging (`git tag v0.1.0 && git push --tags`); GoReleaser builds cross-platform
  binaries and stamps `--version` via ldflags.
