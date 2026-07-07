# kubeloop

**A read-only Kubernetes rightsizing CLI that ranks request waste in dollars and keeps the safety caveats visible.**

```bash
go build -o bin/kubeloop ./cmd/kubeloop
./bin/kubeloop --from-file examples/offline-input.json --cloud aws
```

```
WORKLOAD         CURRENT      PROPOSED     $/MONTH  CONF
recommendations  4000m/4.0Gi  1080m/1.0Gi  $150.49  high
checkout-api     2000m/512Mi  576m/428Mi   $32.48   high
search           1000m/2.0Gi  420m/628Mi   $17.38   med

Estimated waste: $200.34/month across 3 workloads.
  ! search: JVM: memory request is heap-configured, not usage-driven — treat the memory number as a caution
  -> realized when nodes consolidate (Cluster Autoscaler / Karpenter)

Excluded:
  - nightly: batch workload (CronJob) — bursty by design, request-sizing doesn't apply
  - new-svc: only 3d usage history (<7d) — not enough signal
```

Current v0.1 is offline and read-only: it takes workload input from JSON, computes conservative request recommendations, ranks directional monthly waste, and can emit text or a stable JSON schema. The live Kubernetes/Prometheus reader and PR engine are next layers, not hidden side effects.

---

## Already using KRR? Good — keep it.

[Robusta KRR](https://github.com/robusta-dev/krr) is excellent at computing the right numbers, and kubeloop can use it as a future recommendation engine. What kubeloop is building around the numbers:

**KRR tells you the right numbers. kubeloop ranks the waste today; the next layers get fixes merged and prove savings.**

- **Dollars, ranked.** Millicores don't get budget approved; "$131/month on checkout-api" does.
- **Through Git, not around it.** Some tools patch pods via admission webhook (your manifest says 2000m, the pod runs 492m — Git is now lying). Others change the cluster directly and your GitOps controller reverts them on the next sync. The planned PR engine will open reviewed, versioned, audited changes against your repo. AWS published this exact PR-based pattern as the GitOps-safe approach — kubeloop is building that pattern as one binary instead of a five-service pipeline.
- **Proof, not promises.** The planned hosted tier measures your bill before and after each merged PR and keeps a verified-savings ledger.

Full comparison: [KRR vs kubeloop](docs/krr-vs-kubeloop.md) · [OpenCost vs kubeloop](docs/opencost-vs-kubeloop.md)

---

## Where the money comes from (the honest version)

- **On GKE Autopilot**, you're billed per pod *request* — so right-sizing cuts the bill **immediately**.
- **On standard clusters (EKS/GKE Standard/AKS)**, pod rightsizing frees capacity, and the savings land **when your nodes consolidate** (Cluster Autoscaler / Karpenter scaling down). kubeloop labels which kind of savings you're looking at; the planned hosted ledger verifies at the bill level — because that's the only level that's real.
- Dollar figures use published on-demand rates (editable `pricing.json` via `--pricing-file`) — treat them as **directional for prioritization**. The *ranking* is the point.

## How it stays safe

One bad recommendation that OOM-kills a production pod would end this project's credibility, so the floors are in code, not convention:

- Proposed CPU is never below **P99 usage × 1.2**; proposed memory never below **max observed + buffer**.
- CronJobs/Jobs and workloads with <7 days of history are **excluded by default**, with the reason printed.
- Memory-sensitive runtimes (JVM etc.) get a caution flag, not a confident number.
- The CLI is **read-only**. The only write path is a pull request that a human reviews and merges.

## Build

```bash
go build -o bin/kubeloop ./cmd/kubeloop
```

## Usage

```bash
./bin/kubeloop --from-file examples/offline-input.json
./bin/kubeloop --from-file examples/offline-input.json --json
./bin/kubeloop --from-file examples/offline-input.json --cloud gcp
./bin/kubeloop --from-file examples/offline-input.json --pricing-file pricing.json
./bin/kubeloop --from-file examples/offline-input.json --per-request
./bin/kubeloop pr --from-file examples/offline-input.json --manifest examples/checkout-deployment.yaml --namespace shop --workload checkout-api --container app --out /tmp/checkout-deployment.patched.yaml
```

Input is currently offline JSON shaped like [`examples/offline-input.json`](examples/offline-input.json). The offline `pr` subcommand prepares one patched manifest file and prints a PR title/body; it does not create branches, call GitHub, or touch the cluster. A future live read-layer will replace `--from-file` with kubeconfig and Prometheus collection. Read-only RBAC manifest for that path: [`deploy/rbac.yaml`](deploy/rbac.yaml).

## FAQ

**How is this different from KRR?** KRR computes recommendations; kubeloop turns recommendations into dollar-ranked reports today, with merged and verified Git changes planned next. Run both — see the [comparison](docs/krr-vs-kubeloop.md).

**Does my data leave the cluster?** No. Current v0.1 reads a local JSON file and prints locally. The future live scan will run against your APIs locally; no phone-home.

**Why PRs instead of auto-apply?** Because your GitOps controller reverts in-cluster changes, and webhook workarounds make Git lie about what's running. A PR is the change mechanism your team already trusts.

**Is there a paid version?** The CLI is free and stays free — never a trial. A hosted tier (continuous scanning, auto-PRs under your policy, verified-savings ledger, multi-cluster) exists for teams that want the loop run for them.

## Roadmap

- [x] Read-only scan: dollar-ranked CPU/memory waste
- [x] Offline PR preparation: raw YAML patch + reviewer-facing title/body
- [ ] Live Kubernetes + Prometheus read-layer
- [ ] GitHub PR creation and Helm/Kustomize source mapping
- [ ] Hosted: continuous scans, policy-gated auto-PRs, weekly digest
- [ ] Verified-savings ledger (before/after, bill-level)
- [ ] Idle PersistentVolume detection · GPU request scanning

## Contributing

The sizing floors live in [`internal/rightsizing`](internal/rightsizing). If the headroom logic is wrong for your workload type, open an issue **with the numbers** — making this trustworthy across stateful, batch, and JVM workloads is the whole game.

## License

MIT. Use it, read it, fork it.
