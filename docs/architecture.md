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
| `internal/readlayer` | Composes inventory + usage into `scan.Input`. Home of the future live cluster reader. | inventory, reporting, rightsizing, safety, scan |
| `internal/pr` | PR engine offline core: find the raw YAML source file, verify and patch a target container's request YAML, compose reviewer-facing title/body, and return the prepared PR payload. | yaml.v3 |
| `cmd/kubeloop` | CLI: flags, `--from-file` input, text + explicit-schema `--json` output. | reporting, rightsizing, savings, scan |

## Design rules
- **`rightsizing` is a pure leaf** — no package below it, so the sizing math and floors are the easy thing to audit and test in isolation.
- **`reporting` is the single renderer** — resource formatting and collision labels live in one place; `scan` delegates to it rather than duplicating.
- **`inventory` stays a leaf of the read-layer** — it never imports `scan`; the glue that maps inventory → `scan.Input` lives in `readlayer`.
- **PR writes are source-only** — `internal/pr` prepares patched manifest content and PR text; it never writes to a cluster or opens a network connection.
- **Read-only** — no package writes to a cluster; the only planned write path is a human-reviewed PR.

## Not built yet (environment/dependency-gated)
- **Live read-layer**: the kube API client (`apimachinery` for `resource.Quantity` parsing) and the Prometheus HTTP client + validated PromQL. `inventory` and `promusage` are the offline-proven halves these wrap.
- **PR engine tail**: Helm/Kustomize rendered-to-source mapping and GitHub PR creation. The offline raw-YAML locator, patcher, composer, and prepare step exist in `internal/pr`.
- **Hosted tier**: continuous scans, policy-gated auto-PRs, verified-savings ledger.

## Known limitations (revisit with the live read-layer)
- **Patch baseline**: `kubeloop pr`'s reduce-only decision compares the *scan's* current request against the proposal, not the *manifest's* current value. In the real flow these are the same (scan reads the cluster; GitOps keeps cluster = manifest), so it's safe. Offline, a `--from-file` input and a `--manifest` can diverge, and a manifest already lower than the proposal could be raised. When the live read-layer lands (scan.current == manifest), this collapses; reconciling the two baselines earlier would be speculative.
- **Manifest assumptions**: the patcher/locator handle single-document, 2-space raw-YAML manifests, verify identity before editing, and preserve `limits`/comments/quote-style. Multi-document files and Helm/Kustomize source mapping are the tool-backed tail.
