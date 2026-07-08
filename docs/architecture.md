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
| `internal/readlayer` | Composes inventory + usage into `scan.Input` (`ToScanInput`). Home of the future live cluster reader. | inventory, reporting, rightsizing, safety, scan |
| `internal/pr` | PR engine offline core: find the raw YAML source file, verify and patch a target container's request YAML, compose reviewer-facing title/body, and return the prepared PR payload. | yaml.v3 |
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

## Not built yet (needs a real cluster / token)
- **Live I/O**: the kube API **LIST** call (a client wrapping `kubeparse`) and **validated PromQL** query strings (need a real Prometheus to confirm; `promclient` takes the query as input for exactly this reason).
- **PR engine tail**: Helm/Kustomize rendered-to-source mapping and GitHub PR creation. The offline raw-YAML locator, patcher, composer, prepare, and guards exist in `internal/pr`.
- **Hosted tier**: continuous scans, policy-gated auto-PRs, verified-savings ledger.

## Known limitations (revisit with the live read-layer)
- **Patch baseline**: `kubeloop pr`'s reduce-only decision compares the *scan's* current request against the proposal, not the *manifest's* current value. In the real flow these are the same (scan reads the cluster; GitOps keeps cluster = manifest), so it's safe. Offline, a `--from-file` input and a `--manifest` can diverge, and a manifest already lower than the proposal could be raised. When the live read-layer lands (scan.current == manifest), this collapses; reconciling the two baselines earlier would be speculative.
- **Manifest assumptions**: the patcher/locator handle single-document, 2-space raw-YAML manifests, verify identity before editing, and preserve `limits`/comments/quote-style. Multi-document files and Helm/Kustomize source mapping are the tool-backed tail.
