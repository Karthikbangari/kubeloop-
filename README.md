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

Current v1.0 is a local, read-only scanner plus a GitHub PR workflow: it can read pre-assembled JSON, raw Kubernetes manifest exports, or a live cluster via `kubectl get` + Prometheus; it computes conservative request recommendations, ranks directional monthly waste, and can open a human-reviewed pull request. It never writes to a cluster.

---

## Already using KRR? Good — keep it.

[Robusta KRR](https://github.com/robusta-dev/krr) is excellent at computing the right numbers, and kubeloop can use it as a future recommendation engine. What kubeloop is building around the numbers:

**KRR tells you the right numbers. kubeloop ranks the waste, opens the fix as a PR, and leaves bill-level proof to the hosted loop.**

- **Dollars, ranked.** Millicores don't get budget approved; "$131/month on checkout-api" does.
- **Through Git, not around it.** Some tools patch pods via admission webhook (your manifest says 2000m, the pod runs 492m — Git is now lying). Others change the cluster directly and your GitOps controller reverts them on the next sync. kubeloop opens reviewed, versioned, audited changes against your repo. AWS published this exact PR-based pattern as the GitOps-safe approach — kubeloop packages that pattern as one binary instead of a five-service pipeline.
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

## Install

Download a prebuilt binary for your platform from the [latest release](https://github.com/Karthikbangari/kubeloop-/releases/latest) (Linux, macOS, and Windows; amd64 and arm64), then extract and put it on your `PATH`:

```bash
# macOS/Linux example — adjust the version, OS, and arch to the asset name
tar -xzf kubeloop_1.0.0_darwin_arm64.tar.gz
sudo mv kubeloop /usr/local/bin/
kubeloop --version
```

Or install from source with Go 1.23+:

```bash
go install github.com/Karthikbangari/kubeloop-/cmd/kubeloop@latest
```

Or build from a clone:

```bash
git clone https://github.com/Karthikbangari/kubeloop-.git
cd kubeloop-
go build -o bin/kubeloop ./cmd/kubeloop
./bin/kubeloop --version
```

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

### Scanning a live cluster (`--from-cluster`)

Reads workloads from your current cluster and their usage from Prometheus. **Read-only** — the only verb kubeloop ever passes to `kubectl` is `get`, and it never writes to the cluster.

```bash
kubectl port-forward -n monitoring svc/prometheus 9090:9090 &
./bin/kubeloop scan --from-cluster --prometheus http://localhost:9090
./bin/kubeloop scan --from-cluster --prometheus http://localhost:9090 --namespace shop --context prod
```

Requirements: `kubectl` on `PATH` (kubeloop reuses your kubeconfig auth, including EKS/GKE/AKS exec plugins), and a Prometheus scraping cadvisor/kubelet metrics — `kube-prometheus-stack` works out of the box. Least-privilege RBAC: [`deploy/rbac.yaml`](deploy/rbac.yaml).

Honest expectations:

- A workload needs **≥7 days of usage history** to be sized. Newer ones are listed under `Excluded` with a reason, never sized on thin data.
- A workload Prometheus has no data for is excluded with a reason — never assumed idle.
- If Prometheus is **unreachable, the scan fails loudly**. It will not report `$0.00 waste`, which would read as "nothing to save."

### Scanning real manifests (`--from-manifests`)

Point kubeloop at a directory of Kubernetes manifests (JSON, as from `kubectl get -o json`) and a usage export, and it reads the current requests straight from the manifests — no cluster access:

```bash
./bin/kubeloop scan --from-manifests examples/manifests --usage-file examples/manifests-usage.json
```

```
WORKLOAD      CURRENT      PROPOSED    $/MONTH  CONF
checkout-api  2000m/512Mi  576m/428Mi  $32.48   high
search        1000m/2.0Gi  420m/628Mi  $17.38   med

Estimated waste: $49.85/month across 2 workloads.
  ! search: JVM: memory request is heap-configured, not usage-driven — treat the memory number as a caution
  -> realized when nodes consolidate (Cluster Autoscaler / Karpenter)
```

The usage export is a JSON map keyed by `namespace/name` — see [`examples/manifests-usage.json`](examples/manifests-usage.json):

```json
{
  "shop/checkout-api": { "P95CPU": 410, "P99CPU": 480, "MaxMem": 314572800, "HistoryDays": 30 }
}
```

A workload with no usage entry is **excluded with a printed reason**, never sized on no data. A typo in the usage file is a hard error, not a silently-dropped field.

### Opening the fix as a pull request (`pr --open`)

`kubeloop pr` patches one workload's requests in your manifest. With `--out` it writes the patched file locally. With `--open` it branches, commits, pushes, and opens a pull request:

```bash
# See exactly what would happen. No token needed, nothing changes.
kubeloop pr --from-file scan.json --manifest deploy.yaml \
  --workload checkout-api --container app --open --dry-run

export GITHUB_TOKEN=ghp_...   # needs `repo` scope
kubeloop pr --from-file scan.json --manifest deploy.yaml \
  --workload checkout-api --container app --open
```

What it will and won't do:

- **Never writes to your cluster.** The only writes are one file in your checkout, one branch, one commit, one push, one PR.
- **Refuses a dirty working tree**, so your uncommitted work is never swept into kubeloop's PR.
- Commits **only** the patched manifest — never `git add .`.
- **Never pushes to the base branch.** The branch is `kubeloop/rightsize-<ns>-<name>-<hash-of-the-patch>`: re-running the same proposal reuses it rather than littering your remote.
- Refuses a manifest outside `--repo-dir`, and refuses to open a PR that changes nothing.
- If the push succeeds but the PR call fails, it **tells you the branch was pushed** rather than leaving you to find it.

Input can also be pre-assembled scan JSON shaped like [`examples/offline-input.json`](examples/offline-input.json).

## FAQ

**How is this different from KRR?** KRR computes recommendations; kubeloop turns recommendations into dollar-ranked reports and opens the fix as a reviewed pull request. Run both — see the [comparison](docs/krr-vs-kubeloop.md).

**Does my data leave the cluster?** No. kubeloop reads from your cluster and Prometheus over your existing local credentials and prints locally; there is no phone-home. `--from-cluster` only ever runs `kubectl get` (read-only), and the only write path is a pull request you review.

**Why PRs instead of auto-apply?** Because your GitOps controller reverts in-cluster changes, and webhook workarounds make Git lie about what's running. A PR is the change mechanism your team already trusts.

**Is there a paid version?** The CLI is free and stays free — never a trial. A hosted tier (continuous scanning, auto-PRs under your policy, verified-savings ledger, multi-cluster) exists for teams that want the loop run for them.

## Roadmap

- [x] Read-only scan: dollar-ranked CPU/memory waste
- [x] Offline PR preparation: raw YAML patch + reviewer-facing title/body
- [x] Live Kubernetes + Prometheus read-layer (`--from-cluster`)
- [x] GitHub PR creation (`pr --open`)
- [ ] Helm/Kustomize source mapping (today the patcher targets raw YAML manifests)
- [ ] Hosted: continuous scans, policy-gated auto-PRs, weekly digest
- [ ] Verified-savings ledger (before/after, bill-level)
- [ ] Idle PersistentVolume detection · GPU request scanning

## Contributing

The sizing floors live in [`internal/rightsizing`](internal/rightsizing). If the headroom logic is wrong for your workload type, open an issue **with the numbers** — making this trustworthy across stateful, batch, and JVM workloads is the whole game.

## License

MIT. Use it, read it, fork it.
