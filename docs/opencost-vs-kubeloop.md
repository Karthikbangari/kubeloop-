# OpenCost vs kubeloop: which one should you actually use?

Short answer: **if you have engineering time to build dashboards and just need cost data, use OpenCost — it's free, it's a CNCF standard, and it's excellent at what it does.** If you want something to tell you, in dollars, exactly which workloads to resize and what to set them to — without building that layer yourself — that's the gap kubeloop fills.

This page is deliberately honest, including about when *not* to use kubeloop. If we push you toward a paid thing you don't need, we've lost the trust this whole project runs on.

> Looking for the rightsizing-tool comparison instead? That's [KRR vs kubeloop](krr-vs-kubeloop.md) — KRR computes recommendations; kubeloop merges them as GitOps PRs and proves the dollars. This page covers the cost-*visibility* layer (OpenCost), which is a different job again.

---

## What each one actually is

**OpenCost** is a cost *allocation and visibility* layer. It answers "where is my money going?" — cost by namespace, workload, label, team. It's the open-source engine underneath several commercial products. It is vendor-neutral, self-hosted, and free. Its documented limitation is that it's strictly a data layer: no built-in analysis of request-vs-usage, no rightsizing suggestions, no ready-made dashboards — you build the reporting on top.

**kubeloop** is a *waste-detection and rightsizing* tool. It answers "which workloads are over-provisioned, how many dollars is that, and what should I set instead?" It reads your requests, compares them to actual P99 usage, converts the gap to money, and hands you a safe suggested value. Read-only, self-hosted, free CLI.

They are not really competitors. **OpenCost tells you where money goes. kubeloop tells you where money is wasted and what to change.** Plenty of teams run both.

---

## Side by side

| | OpenCost | kubeloop |
|---|---|---|
| Core job | Cost allocation & visibility | Waste detection & rightsizing |
| Answers | "Where does spend go?" | "What's over-provisioned, and what should it be?" |
| Request-vs-usage analysis | You build it | Built in |
| Dollar-ranked rightsizing suggestions | No | Yes |
| Dashboards | You build them | CLI output (hosted UI in progress) |
| Changes your cluster | No | No (read-only; hosted tier opens PRs you approve) |
| Data leaves cluster | No | No |
| Cost | Free (CNCF) | Free CLI |
| Setup time | Moderate + build reporting | Minutes |
| Best for | FinOps teams needing allocation & chargeback | Teams that want to *act* on waste fast |

---

## When to use OpenCost (and not kubeloop)

- You need **chargeback/showback** — attributing spend to teams for internal billing. That's OpenCost's home turf; kubeloop doesn't do allocation.
- You already have engineers who enjoy building Grafana dashboards and want full control of the data model.
- You want the CNCF-standard cost data feeding your own tooling.

In these cases OpenCost is the right call and kubeloop adds little.

## When to use kubeloop (and not just OpenCost)

- You want a **dollar figure and a specific fix** today, not a data layer to build on.
- You're a **small or mid-size team with no FinOps person** — the market's own guides suggest teams under ~$10k/month either buy an enterprise tool they can't self-serve or use OpenCost and build everything themselves. kubeloop is the third option those guides skip.
- You want the analysis done *for* you, safely, without routing production data to a vendor cloud.

## When to use both

Common and sensible: OpenCost for allocation and chargeback, kubeloop for the request-vs-usage rightsizing loop. They read the same cluster and answer different questions.

---

## The honest catch

kubeloop's dollar figures use published cloud rates and are meant for *prioritization* — the ranking of waste is more precise than the absolute number. If you need billing-grade cost accuracy reconciled against your actual invoice, that's OpenCost/Kubecost territory. We're optimizing for "which five things should I fix first," not for the finance team's ledger.

And the honest self-interested note: the kubeloop CLI is free and stays free. There's a hosted version for teams that want the loop run continuously — auto-generated pull requests, verified before/after savings, multi-cluster. If the CLI is all you need, use just the CLI. We'd rather you trust the free tool than resent a paywall.

---

## Try it

```bash
brew install kubeloop
kubeloop scan --prometheus-url http://localhost:9090
```

One command, read-only, and you'll see your waste in dollars in under a minute. If OpenCost already covers you, keep using it — and star the repo in case the rightsizing loop becomes useful later.
