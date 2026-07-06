<!-- SPEC DRAFT — this is the build target. Before publishing: replace example output with REAL tool output, verify install commands work. Finalize in the Packaging slice. -->

# kubeloop

**Turn Kubernetes rightsizing into merged pull requests — with the savings in dollars, proven on your bill.**

```bash
kubeloop scan
```

```
Scanning cluster "prod-gke-1"...

WORKLOAD           REQUESTED    P99 USED   WASTED    $/MONTH   PROPOSE   CONF
──────────────────────────────────────────────────────────────────────────────
checkout-api       2000m CPU    410m       1508m     $131.20   492m      high
recommendations    4Gi MEM      900Mi      2.9Gi     $ 78.40   1.1Gi     high
image-resizer      1500m CPU    180m       1284m     $111.70   216m      high
notifications      2Gi MEM      340Mi      1.5Gi     $ 41.30   408Mi     med
...

💸  Wasting an estimated $1,240/month across 23 workloads.
    Open a fix as a pull request:  kubeloop pr checkout-api
```

```bash
kubeloop pr checkout-api
```

```
✔ Located source: helm/values/prod.yaml → checkoutApi.resources.requests
✔ Opened PR #214: "Right-size checkout-api: save ~$131/month"
  Evidence, confidence, and rollback note included. You review, you merge.
```

Read-only until *you* merge. Nothing leaves your cluster.

---

## Already using KRR? Good — keep it.

[Robusta KRR](https://github.com/robusta-dev/krr) is excellent at computing the right numbers, and kubeloop can use it as its recommendation engine. What kubeloop adds is everything *after* the numbers:

**KRR tells you the right numbers. kubeloop gets them merged and proves the savings.**

- **Dollars, ranked.** Millicores don't get budget approved; "$131/month on checkout-api" does.
- **Through Git, not around it.** Some tools patch pods via admission webhook (your manifest says 2000m, the pod runs 492m — Git is now lying). Others change the cluster directly and your GitOps controller reverts them on the next sync. kubeloop opens a pull request against your repo: reviewed, versioned, audited, and *true*. AWS published this exact PR-based pattern as the GitOps-safe approach — kubeloop is that pattern as one binary instead of a five-service pipeline.
- **Proof, not promises.** The hosted tier measures your bill before and after each merged PR and keeps a verified-savings ledger.

Full comparison: [KRR vs kubeloop](docs/krr-vs-kubeloop.md) · [OpenCost vs kubeloop](docs/opencost-vs-kubeloop.md)

---

## Where the money comes from (the honest version)

- **On GKE Autopilot**, you're billed per pod *request* — so every merged right-sizing PR cuts the bill **immediately**. If you're on Autopilot, this tool pays for itself the week you install it.
- **On standard clusters (EKS/GKE Standard/AKS)**, pod rightsizing frees capacity, and the savings land **when your nodes consolidate** (Cluster Autoscaler / Karpenter scaling down). kubeloop labels which kind of savings you're looking at, and the hosted ledger verifies at the bill level — because that's the only level that's real.
- Dollar figures use published on-demand rates (editable `pricing.yaml`) — treat them as **directional for prioritization**. The *ranking* is the point.

## How it stays safe

One bad recommendation that OOM-kills a production pod would end this project's credibility, so the floors are in code, not convention:

- Proposed CPU is never below **P99 usage × 1.2**; proposed memory never below **max observed + buffer**.
- CronJobs/Jobs and workloads with <7 days of history are **excluded by default**, with the reason printed.
- Memory-sensitive runtimes (JVM etc.) get a caution flag, not a confident number.
- The CLI is **read-only**. The only write path is a pull request that a human reviews and merges.

## Install

```bash
brew install kubeloop        # or:
curl -sSL https://get.kubeloop.dev | sh
```

## Usage

```bash
kubeloop scan --prometheus-url http://localhost:9090   # dollar-ranked waste table
kubeloop scan --json                                    # machine output
kubeloop pr <workload> --repo github.com/you/infra      # open the fix as a PR
```

Requires: a kubeconfig with read access, Prometheus (or compatible), and — for `pr` — a GitHub token. Read-only RBAC manifest: [`deploy/rbac.yaml`](deploy/rbac.yaml).

## FAQ

**How is this different from KRR?** KRR computes recommendations; kubeloop turns recommendations (KRR's or its own) into dollar-ranked reports and merged, verified Git changes. Run both — see the [comparison](docs/krr-vs-kubeloop.md).

**Does my data leave the cluster?** No. The scan runs against your APIs and prints locally. No phone-home.

**Why PRs instead of auto-apply?** Because your GitOps controller reverts in-cluster changes, and webhook workarounds make Git lie about what's running. A PR is the change mechanism your team already trusts.

**Is there a paid version?** The CLI is free and stays free — never a trial. A hosted tier (continuous scanning, auto-PRs under your policy, verified-savings ledger, multi-cluster) exists for teams that want the loop run for them.

## Roadmap

- [x] Read-only scan: dollar-ranked CPU/memory waste
- [x] `kubeloop pr`: rightsizing as a pull request (raw YAML, Kustomize; Helm best-effort)
- [ ] Hosted: continuous scans, policy-gated auto-PRs, weekly digest
- [ ] Verified-savings ledger (before/after, bill-level)
- [ ] Idle PersistentVolume detection · GPU request scanning

## Contributing

The sizing floors live in [`internal/rightsizing`](internal/rightsizing). If the headroom logic is wrong for your workload type, open an issue **with the numbers** — making this trustworthy across stateful, batch, and JVM workloads is the whole game.

## License

MIT. Use it, read it, fork it.
