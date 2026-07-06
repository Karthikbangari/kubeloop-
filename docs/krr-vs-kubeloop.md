<!-- SPEC DRAFT — verify all KRR claims against your own Phase-1 teardown output before publishing. -->

# KRR vs kubeloop: run both (seriously)

Short answer: **KRR is the best open-source tool for computing what your Kubernetes requests *should* be. kubeloop is the tool for getting those numbers merged into your repo and proving the dollars on your bill.** They're different layers. Most kubeloop users should keep — or start — using KRR.

This page is honest to a fault, including about when kubeloop adds nothing. That honesty is the product's whole trust model, so if we oversell here, everything else falls.

---

## What each layer does

**[Robusta KRR](https://github.com/robusta-dev/krr)** analyzes Prometheus usage history and recommends CPU/memory requests and limits — no in-cluster agent, broad Prometheus-variant support, customizable strategies, a k9s plugin, Slack reports, and an optional Enforcer for auto-applying. Its output speaks in resource values and priorities. It is genuinely good at the *recommendation* layer.

**kubeloop** takes recommendations (from KRR, or its own built-in engine) and handles the *action-and-proof* layer:
1. converts them to **dollars per month, ranked** — the format budget conversations actually run on;
2. turns the fix into a **pull request against your GitOps repo** — located in your Helm values or Kustomize overlays, evidenced, reviewed by a human, merged like any other change;
3. in the hosted tier, **verifies the savings** on your actual bill, before vs after each merged PR.

## The GitOps question (the real difference)

If you run ArgoCD or Flux, Git is your source of truth, and that's exactly where "applying recommendations" gets awkward:

- **Apply in-cluster** (KRR Enforcer, autonomous platforms): your GitOps controller sees drift and reverts the change on the next sync — or you disable drift correction for those fields, weakening the guarantee you adopted GitOps for.
- **Patch at admission** (webhook approaches): the pod runs the optimized values while the manifest still says the old ones. It works — but Git now lies about production, which audit-minded and regulated teams reject. AWS's own architecture guidance names direct patching as breaking Git as the source of truth.
- **Through Git** (kubeloop, and AWS's published reference pattern): the change is a PR. Reviewed, versioned, revertable, true. Slower than a webhook by one code review — which is precisely the feature.

If Git-truthfulness doesn't matter to your team, kubeloop's PR path matters less to you. That's a legitimate position; see "when KRR alone is enough" below.

## Side by side

| | KRR | kubeloop |
|---|---|---|
| Core layer | Recommendation | Action + proof |
| Output speaks in | Resource values, priorities | Dollars/month, ranked |
| Gets changes into Git | No (Enforcer applies in-cluster; SaaS offers YAML snippets) | Yes — locates source in Helm/Kustomize/YAML, opens the PR |
| Verifies savings on the bill | No | Hosted tier: before/after ledger per merged PR |
| Recommendation math | Battle-tested, customizable strategies | Uses KRR as an engine, or built-in conservative percentiles |
| Runs read-only, data stays local | Yes | Yes |
| Cost | Free (OSS; optional SaaS) | Free CLI (OSS; optional hosted tier) |

## When KRR alone is enough

- You just want to *know* the right numbers and will apply them manually at your own pace.
- You don't run GitOps, or you're comfortable with in-cluster application.
- You want Robusta's SaaS ecosystem around your recommendations.

In those cases, use KRR and skip kubeloop — genuinely.

## When kubeloop earns its place

- Your recommendations rot unapplied because "who edits the Helm values and opens the PR" is nobody's job. (This is the most common failure mode in rightsizing, and it's a *workflow* problem, not a math problem.)
- You need the change history in Git for audit, review, or regulated environments.
- You need dollars to prioritize and to justify the work — especially on **GKE Autopilot**, where per-request billing means a merged PR cuts the bill immediately.
- You want savings *proven* on the bill, not estimated in a dashboard.

## The honest catches

- kubeloop's dollar figures use list prices (editable) — **directional for ranking**, not billing-grade. Tools that reconcile against your negotiated rates will show different absolute numbers.
- kubeloop's built-in engine is deliberately conservative percentile math. KRR's strategies are more configurable, and commercial ML tools (per-workload seasonal models) are more accurate still. kubeloop's claim is workflow and trust, not superior math — which is exactly why using KRR *as* the engine is a first-class option.
- On non-Autopilot clusters, merged PRs save money **when nodes consolidate** — pair kubeloop with Cluster Autoscaler or Karpenter, or the freed capacity just sits there cheaper-looking but not cheaper.

## Try the pair

```bash
# the numbers
krr simple

# the numbers → dollars → merged change
kubeloop scan
kubeloop pr <workload>
```

If KRR already covers you end-to-end, star the repo and move on — no hard feelings. If your recommendations keep dying in a backlog, that's the exact gap this exists for.
