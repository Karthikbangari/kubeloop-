# kubeloop — handoff

_Snapshot: 2026-07-08. Live at github.com/Karthikbangari/kubeloop- (`main`, 24 commits, all pushed)._

## What it is
A read-only Kubernetes rightsizing CLI (single Go binary, `github.com/kubeloop/kubeloop`).
It scans workloads, ranks wasted CPU/memory requests **in dollars**, and prepares the fix
as a **reviewed pull request** — it never writes to a cluster. Pitch: *"KRR tells you the
right numbers; kubeloop ranks the waste and gets it merged."* See `README.md` and
`plan/MASTER-PLAN-LOOPED.md`.

## Status: feature-complete. Two things have never run against the real world.
Every planned code milestone is built, tested, graduated, and on `main`. `playground/` is empty.

- `kubeloop scan --from-cluster --prometheus URL` → reads the **live cluster** (read-only `kubectl get`)
  and Prometheus, ranks waste. Validated against kind + kube-prometheus-stack (RULEBOOK #77).
- `kubeloop pr --open` → branches, commits the patched manifest, pushes, opens a **real pull request**.
  Local git half validated for real; the GitHub POST has never run.

**The two unvalidated gaps — read these before trusting output:**
1. **The GitHub POST has never reached github.com.** `internal/pr/ghclient` is httptest-only. Needs a
   `repo`-scoped `GITHUB_TOKEN` and a *scratch* repo (not `kubeloop-` itself) to prove.
2. **7-day windowing is unproven.** The live queries were validated on a minutes-old kind cluster, so
   metric names, labels, and the pod selector are confirmed, but `[7d:5m]` behaviour is not. Needs a
   cluster with a week of history.

Also: RULEBOOK entries **#64–#70, #75, #76, #80, #81** are live code that Codex has not reviewed.

## What still works offline

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
  names, no-op/under-provisioned targets. Swap `--out` for `--open` to open a real PR
  (`--dry-run` shows the plan and needs no token).
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
Note: Go uses its default caches (`go env GOCACHE` → `~/Library/Caches/go-build`). An earlier setup
kept workspace-local `.gocache/`/`.gomodcache/`; those went stale and were deleted. Nothing sets
`GOCACHE`/`GOMODCACHE` — not the Makefile, not CI.

To exercise the live paths you need a cluster and a Prometheus:
```bash
go install sigs.k8s.io/kind@latest && kind create cluster --name kubeloop
helm install kps prometheus-community/kube-prometheus-stack -n monitoring --create-namespace
kubectl port-forward -n monitoring svc/kps-kube-prometheus-stack-prometheus 9090:9090 &
./bin/kubeloop scan --from-cluster --prometheus http://localhost:9090
kind delete cluster --name kubeloop   # when done
```

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

## Done since the last snapshot (all 3 code milestones)
- ✅ **Read-layer graduated** (#71) → `internal/readlayer/*`, `--from-manifests` shipped.
- ✅ **Live cluster reader** (#72–#74, #77, #79) → `--from-cluster`. Shells out to `kubectl get`
  rather than taking a client-go dependency: `kubeparse` already consumes that output, and kubectl
  inherits kubeconfig auth (EKS/GKE/AKS exec plugins). Validated on a real kind cluster, which
  **found a real bug**: `HistoryDays` measured the longest-lived *pod*, not the workload, so any
  service that deploys daily would report ~1 day of history and be silently excluded as "<7d".
- ✅ **PR engine** (#75, #76, #80, #81) → `pr --open`. Local git validated against real git; the
  GitHub POST is httptest-only.

## What's left (2 validations + review debt — no unbuilt code)
1. **Prove the GitHub POST.** Needs `export GITHUB_TOKEN=ghp_…` (`repo` scope) and a **scratch repo**
   — do not test-drive PR creation against `kubeloop-` itself. Then: `pr --open` once, verify the
   diff, close the PR. `--dry-run` already works with no token.
2. **Prove 7-day windowing.** Needs a cluster that has been running a week. A fresh kind cluster
   confirms metric names, labels, and the pod selector, but every workload is (correctly) excluded
   as "<7d of history", so `[7d:5m]` behaviour stays unproven.
3. **Review debt.** #64–#70, #75, #76, #80, #81 are live code Codex has not reviewed. Codex *is*
   active — it caught a real `NaN`/`Inf` parsing bug in #78 — so pointing it at `internal/pr/*` is
   the last thing before v1.0.

Then tag v1.0.0. Plan "loops" B/C/D (launch, revenue, expansion) are business, not code.

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
