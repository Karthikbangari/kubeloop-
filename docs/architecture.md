# kubeloop architecture

A single Go binary. Packages are layered so the trust-critical core stays a
pure, dependency-free leaf and everything composes toward the CLI. Arrows show
"imports" (dependency direction); there are no cycles.

```
cmd/kubeloop ──────────────► scan ──► reporting ──► labels
     │                        │  │        │
     │                        │  ├──► safety ──┐
     │                        │  └──► savings  │
     │                        └─────────────┐  │
     ▼                                       ▼  ▼
  reporting, savings                      rightsizing  (pure leaf: domain types,
                                                        recommender, safety floors)

readlayer ──► inventory ──► rightsizing        (read-layer, offline halves)
    └───────► promusage ──► rightsizing
    └───────► scan (assembles inventory+usage → scan.Input)

readlayer/dirsource ──► manifestsource ──► kubeparse ──► quantityparse
                              └──► readlayer.ToScanInput   (offline manifest read path:
readlayer/promclient ──► promusage                         manifests + usage → scan.Input)

pr ──► yaml.v3                              (PR-engine offline path:
                                             locate raw YAML + patch + PR text)
```

## Packages

| Package | Responsibility | Imports |
|---|---|---|
| `internal/rightsizing` | Domain types (`Usage`, `Resources`, `Price`), the `Percentile` recommender, and the safety floors (CPU ≥ P99×1.2, mem ≥ max+buffer) + `MonthlyWaste`. **The trust core.** | — (leaf) |
| `internal/labels` | Collision-aware `namespace/name` labeling shared by the table and excluded list. | — (leaf) |
| `internal/savings` | Bill-realization wording (immediate vs on node consolidation). | — (leaf) |
| `internal/safety` | Exclusions (CronJob/Job, <7d history) with reasons, confidence scoring, JVM caution. | rightsizing |
| `internal/reporting` | Cloud pricing (+`--pricing-file` override), dollar-ranking, and the one table renderer (CONF column, cautions, labels). | labels, rightsizing |
| `internal/scan` | Orchestrator: assess → exclude → rank → score → render (`Report`). | labels, reporting, rightsizing, safety, savings |
| `internal/inventory` | Read-layer: effective pod request (`max(sum regular, max init)`) and JVM runtime detection. Numeric-in (quantity parsing is the live client's job). | rightsizing |
| `internal/readlayer/promusage` | Read-layer: parse Prometheus instant-query JSON → scalar, cores→millicores, assemble `Usage`. | rightsizing |
| `internal/readlayer/promclient` | Read-layer: read-only Prometheus `/api/v1/query` GET + parse. Query *construction* is the caller's job (needs a live Prometheus to validate). | promusage |
| `internal/readlayer/quantityparse` | Read-layer: `"2000m"`/`"512Mi"` → millicores/bytes, correct-or-error. Inverse of `internal/pr/quantity`; no `apimachinery`. | — (leaf) |
| `internal/readlayer/kubeparse` | Read-layer: serialized Deployment/StatefulSet JSON → identity + `inventory.Container`s. Malformed quantity is an error, never a silent zero. | inventory, quantityparse |
| `internal/readlayer/manifestsource` | Read-layer: one manifest + usage → `scan.Input`, via `readlayer.ToScanInput`. | readlayer, kubeparse, rightsizing, scan |
| `internal/readlayer/dirsource` | Read-layer: many manifests + a `namespace/name` usage map → `[]scan.Input`. Backs `--from-manifests`. | kubeparse, manifestsource, rightsizing, scan |
| `internal/readlayer/kubeclient` | Live read-layer: lists Deployments/StatefulSets by shelling out to `kubectl get -o json`. Read-only by construction — the only verb ever passed is `get`. | kubeparse |
| `internal/readlayer/promql` | Live read-layer: builds the P95/P99 CPU, max-memory, and history query strings + a kind-aware pod selector. Pure strings, no I/O. | — (leaf) |
| `internal/readlayer/clustersource` | Live read-layer: workloads + Prometheus → `[]scan.Input`, via `readlayer.ToScanInput`. Backs `--from-cluster`. | kubeparse, manifestsource, promql, promusage, scan |
| `internal/readlayer` | Composes inventory + usage into `scan.Input` (`ToScanInput`). Home of the future live cluster reader. | inventory, reporting, rightsizing, safety, scan |
| `internal/pr` | PR engine offline core: find the raw YAML source file, verify and patch a target container's request YAML, compose reviewer-facing title/body, and return the prepared PR payload. | yaml.v3 |
| `internal/pr/gitrepo` | Local git via shell-out: resolve origin, refuse a dirty tree, branch, commit one file, push. Never pushes the base branch. | — (leaf) |
| `internal/pr/ghclient` | `POST /repos/{owner}/{repo}/pulls` over `net/http`. Creates pull requests and nothing else. Token never in a URL; scrubbed from every error. | — (leaf) |
| `internal/pr/openpr` | Composes `pr.Prepare` → `gitrepo` → `ghclient`. The only outward-facing side effect in kubeloop. | pr, gitrepo, ghclient |
| `cmd/kubeloop` | CLI: flags, `--from-file` / `--from-manifests` input, text + explicit-schema `--json` output. | pr, readlayer/dirsource, reporting, rightsizing, savings, scan |

## Design rules
- **`rightsizing` is a pure leaf** — no package below it, so the sizing math and floors are the easy thing to audit and test in isolation.
- **`reporting` is the single renderer** — resource formatting and collision labels live in one place; `scan` delegates to it rather than duplicating.
- **`inventory` stays a leaf of the read-layer** — it never imports `scan`; the glue that maps inventory → `scan.Input` lives in `readlayer`.
- **One place assembles a `scan.Input`** — `readlayer.ToScanInput`. Every read source (offline manifests today, the live cluster reader later) funnels through it, so a change to how a workload becomes scan input can't drift between sources.
- **PR writes are source-only** — `internal/pr` prepares patched manifest content and PR text; it never writes to a cluster or opens a network connection.
- **Read-only** — no package writes to a cluster; the only planned write path is a human-reviewed PR.

## Read-layer status (offline halves proven and graduated; live I/O gated)
The read-layer is built, tested offline, and graduated into `internal/readlayer/`
(RULEBOOK.md #71); only the two calls that need a real cluster remain.
- **Prometheus HTTP client** (`promclient`) — read-only `/api/v1/query` GET + parse. Proven with `httptest`.
- **usage parsing** (`promusage`) — Prometheus response → scalar → `rs.Usage`.
- **quantity parsing** (`quantityparse`) — `"2000m"`/`"512Mi"` → millicores/bytes, correct-or-error. Done *without* `apimachinery` (project stays lightweight); the inverse of `internal/pr/quantity`.
- **kube-object parsing** (`kubeparse`) — a serialized Deployment/StatefulSet → `inventory.Container`s.
- **manifest→scan bridge** (`manifestsource`) — composes the above into a `scan.Input`, proven end-to-end into `scan.Scan`.
- **directory source** (`dirsource`) — many manifests + a usage export → `[]scan.Input`. Shipped to users as `kubeloop scan --from-manifests DIR --usage-file USAGE.json`.

This makes the offline manifest path a real product mode, not just a proof: a
GitOps repo's manifests plus a Prometheus usage dump produce a ranked report
with no cluster access at all. A workload with no usage entry is **excluded with
a reason**, never sized on no data.

## Live read-layer (built, graduated, validated against a real cluster)
`kubeloop scan --from-cluster --prometheus URL` reads workloads with `kubectl get`
and usage from Prometheus. Validated on kind + kube-prometheus-stack (RULEBOOK #77):
metric names and labels confirmed, and the kind-aware pod selector demonstrably
prevents a workload from absorbing a sibling's usage (`checkout-api-.*` matched
`checkout-api-v2`'s pod; the real selector did not).

- **`kubectl`, not client-go** — kubeparse already consumes `kubectl get -o json`, and kubectl inherits the user's kubeconfig auth (EKS/GKE/AKS exec plugins). A hosted scanner will need an in-process client.
- **Still unvalidated:** 7-day windowing, which needs a cluster with a week of history.

## PR engine (built; validated against the real GitHub API)
`kubeloop pr --open` branches, commits the patched manifest, pushes, and opens a
pull request. The cluster is never touched; the only writes are one file, one
branch, one commit, one push, one PR.

- `internal/pr/gitrepo` — local git, **validated against real git** (a local bare repo stands in for origin: branch lands with the patch, base byte-for-byte untouched).
- `internal/pr/ghclient` — `POST /repos/{owner}/{repo}/pulls`, plain `net/http`. The 201 success path was validated against a throwaway GitHub repo (RULEBOOK #87).
- `internal/pr/openpr` — the composer. Resolves `origin` before mutating anything; refuses a dirty tree; refuses paths escaping the checkout; pushes before asking GitHub, and reports the pushed branch if the PR call then fails.
- `--dry-run` performs none of it.

### What `pr --open` refuses to do
Each of these is a regression test, and each was found by writing the attack first:

- write outside the checkout — lexically (`../x`), through a **symlinked leaf**, or through a **symlinked parent directory**
- write anywhere inside **`.git`** (matched by path segment, case-insensitively): the patch lands before the commit, so a clobbered `pre-commit` hook would execute
- run on a **dirty working tree**, or in a **detached HEAD**
- `git add .` / `commit -a` — only the one patched file is staged
- push to the **base branch**, or open a PR from a branch onto itself
- open a PR that changes nothing

## Validated against the real GitHub API (RULEBOOK #87)
`kubeloop pr --open` opened a real pull request on a throwaway repository:
1 commit, 1 changed file, +2/−2, base `main` **untouched**, evidence table and
safety-floor disclosure in the body. Re-running the same proposal opened no
second PR — the content-keyed branch collided. Invalid tokens return a real 401
mapped to an actionable message with no token leak.

## Not validated yet
- **7-day windowing** in the live read-layer — needs a cluster with a week of
  history. A fresh cluster confirms metric names, labels, and the pod selector,
  but every workload is (correctly) excluded as "<7d of history", so the
  `[7d:5m]` subquery shape never actually runs.
- **The Windows hard-link guard** (`internal/pr/openpr/hardlink_windows.go`) is
  compile-verified only; it has never executed on Windows. It fails closed.
- **PR engine tail**: Helm/Kustomize rendered-to-source mapping. The raw-YAML locator, patcher, composer, prepare, local git, GitHub PR creation, and guards exist in `internal/pr`.
- **Hosted tier**: continuous scans, policy-gated auto-PRs, verified-savings ledger.

## Known limitations (revisit with the live read-layer)
- **Patch baseline**: `kubeloop pr`'s reduce-only decision compares the *scan's* current request against the proposal, not the *manifest's* current value. In the intended live flow these are the same (scan reads the cluster; GitOps keeps cluster = manifest), so it's safe. Offline, a `--from-file` input and a `--manifest` can diverge, and a manifest already lower than the proposal could be raised.
- **Manifest assumptions**: the patcher/locator handle single-document, 2-space raw-YAML manifests, verify identity before editing, and preserve `limits`/comments/quote-style. Multi-document files and Helm/Kustomize source mapping are the tool-backed tail.
