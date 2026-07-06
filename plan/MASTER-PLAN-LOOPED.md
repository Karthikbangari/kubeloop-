# kubeloop — MASTER PLAN, LOOP EDITION
### The same validated plan, rewritten as a loop machine. This file REPLACES kubeloop-final-master-plan.md — keep only this one.

---

# THE CORE (unchanged, one screen)

**Product:** the open-source tool that turns Kubernetes rightsizing into **merged pull requests with proven dollars**.
**One-liner:** *KRR tells you the right numbers. kubeloop gets them merged and proves the savings.*
**Three differentiators:** Dollars (ranked $/month, not millicores) · Through Git (PRs — StormForge bypasses Git via webhook, ScaleOps/CAST ignore it, AWS published the PR pattern as a DIY blueprint no product owns) · Verified savings (before/after bill ledger per merged PR).
**Validation stack:** KRR proves recommendation demand · AWS blueprint proves the PR pattern is wanted and unproductized · native cloud tools are per-cloud console suggestions · Y Combinator portfolio + five 2026 market maps show nobody owns the niche (closest: Metoro — incident PRs, not cost) · Infracost proved the identical OSS-CLI→PR→cloud-tier motion in the IaC lane.
**Beachheads:** GKE Autopilot teams (per-request billing → rightsizing cuts the bill instantly) · GitOps small teams $2k–15k/month (documented as underserved).
**Honesty rules baked into product + docs:** dollars are "directional" (list prices) · on non-Autopilot clusters savings realize only when nodes consolidate — measure at bill level and say so · accuracy is not the claim (StormForge's ML is better); workflow + trust is the claim.

**Attack lines (memorize):**
"KRR exists" → *"KRR gives numbers; kubeloop merges them and proves dollars. Run both."*
"StormForge handles GitOps" → *"By making Git lie. kubeloop keeps manifest = what runs."*
"GKE console shows this" → *"Per cloud, as suggestions. kubeloop: any cloud, in Git, with a ledger."*
"We'll DIY the AWS blog" → *"Five services and a pipeline vs one binary in ten minutes."*

---

# THE LOOP MACHINE

Everything below is a loop with: **ENTRY** (when it starts) · **CYCLE** (what repeats) · **CADENCE** · **EXIT UP** (graduate) · **EXIT DOWN** (kill → Pivot Loop). Loops nest: the Master Loop contains stage loops; the Forever Loops run underneath once ignited; your Weekly Loop drives all of them.

```
                    ┌────────────── MASTER LOOP ──────────────┐
                    │  BUILD → LAUNCH → MEASURE → DECIDE →    │
                    │  EXPAND → (bigger) → repeat             │
                    └──────────────────────────────────────────┘
   LOOP A          LOOP B           LOOP C           LOOP D
   Foundation  →   Ignition    →    Revenue     →    Expansion
   (build-test)    (seed-measure)   (convert-verify) (upsell-deepen)
        │               │                │                │
        └──── exit-down from any loop ──→ PIVOT LOOP ─────┘
   Forever loops underneath: Runtime · Growth · Money · Evolution
   Driving everything: YOUR WEEKLY LOOP (plan → build → review → decide)
```

---

## YOUR WEEKLY LOOP — the engine that turns all others
**ENTRY:** now. **CADENCE:** every week, forever.
**CYCLE:**
1. Monday (30 min): pick this week's loop iteration + its single success metric.
2. Tue–Sat: 4h build + ≤1h issues/metrics per day. Build only what the current loop's cycle step demands.
3. Friday (15 min): write one line — current loop, metric value, kill-condition status.
4. Decide: iterate again / exit up / exit down. Never decide mid-week on emotion.
**HARD RULES:** read-only until a human merges · under-claim dollars · no new-idea shopping · no further desk research (checked against enterprise vendors, OSS, clouds, accelerator portfolios — the map is done; only terminals produce new information now).

---

## LOOP A — FOUNDATION LOOP (build-test cycles, ~weeks 1–4)
**ENTRY:** today. **CADENCE:** one slice per cycle, 1–5 days each.
**CYCLE (repeat per slice):** pick slice → build on dev cluster → test against hand-computed truth → fails? fix and re-loop → passes? next slice.

**Slice queue (in order):**
1. **Env slice (d1–2):** kind + kube-prometheus-stack + 8–10 deliberately padded workloads (memory-heavy, CronJob, bursty, StatefulSet) wrapped in `make dev-up`. *Pass:* one command → cluster accumulating fake waste.
2. **Teardown slice (d3–5) ⚠️ GATE CYCLE:** run KRR on the dev cluster (capture CLI + `--json` — confirm no per-workload dollars, behavior on bursty/CronJob) → run Goldilocks → simulate the GitOps revert (in-cluster patch on an Argo-managed app) → read the AWS blueprint end-to-end → write POSITIONING.md → choose engine adapter (KRR-ingest vs own PromQL at CPU≈P95, mem=max+15%). *Pass:* two-sentence answer to a hostile HN commenter. *EXIT DOWN:* KRR already dollarizes + PRs → Pivot Loop immediately, zero product code written.
3. **Read-layer slice (d6–9):** read-only ServiceAccount + rbac.yaml → workload inventory (multi-container, init containers, <7d exclusions) → `Recommender` interface + chosen adapter. *Pass:* `scan --raw` JSON matches hand math for 3 workloads.
4. **Dollar slice (d10–12):** pricing.yaml (AWS/GCP/Azure defaults) + node-type detection + waste $ = (current−proposed)×price×replicas×730h → ranked table + headline total → detect GKE Autopilot and label savings "immediate" vs "on node consolidation." *Pass:* every line defensible.
5. **Safety slice (d13–15):** colorized table, --json, --namespace · exclusions with printed reasons · confidence column · floors IN CODE (CPU ≥ P99×1.2, mem ≥ max+buffer) · JVM caution flags. *Pass:* bursty + CronJob produce cautious output, not confident nonsense.
6. **PR-engine slice (d16–20, THE HEART):** manifest locator via the AWS technique (render with `helm template`/`kustomize build`, map rendered workload → source path) → minimal-diff patcher → PR composer titled with the $ figure, evidence + rollback in body → v1 auth = GitHub token env var. Raw YAML + Kustomize solid; Helm best-effort with clear failures. *Pass:* scan → `kubeloop pr` → merge → ArgoCD syncs → next scan shows waste gone.
7. **Packaging slice (d21–23):** goreleaser binaries + brew tap + curl installer → README rewritten to final positioning ($ table + PR step as hero, "data never leaves your cluster," "CLI stays free") → "KRR vs kubeloop (run both)" + "OpenCost vs kubeloop" pages with the node-consolidation honesty paragraph. *Pass:* stranger installs to dollar table in <2 minutes.

**EXIT UP:** v0.1.0 tagged + end-to-end demo clean → Loop B.
**EXIT DOWN:** only via the gate cycle (slice 2).

---

## LOOP B — IGNITION LOOP (seed-measure-iterate, weeks 4–10)
**ENTRY:** v0.1.0 public. **CADENCE:** weekly iterations; hard 6-week clock.
**SEED ONCE (week 1 of the loop, one act/day, never repeated):** repo public with search topics → Show HN leading WITH the KRR relationship → tool-first replies in 3–5 existing r/kubernetes / r/devops waste threads → submissions to awesome-lists + KubeWeekly-style newsletters → metrics dashboard live.
**CYCLE (weekly thereafter):** read metrics (installs, stars, clones, shares) → triage issues → fix the ONE top recurring complaint → ship → re-measure.
**MID-LOOP CHECK (week 2–3):** trending + unsolicited shares → start Loop C in parallel. Flat ~10 stars → freeze Loop C, spend remaining weeks on distribution-only iterations (different channels, README hero, install friction).
**EXIT UP:** ~200 meaningful installs/stars trending by week 6 → Loop C at full speed.
**EXIT DOWN (week 6, no negotiation):** flat AND no organic sharing → distribution failure → Pivot Loop. Never polish a product with a distribution problem.

---

## LOOP C — REVENUE LOOP (connect-convert-verify-charge, weeks 7–12)
**ENTRY:** Loop B mid-check positive. **CADENCE:** weekly build-slices + per-customer cycles.
**BUILD SLICES (weekly):** hosted scaffolding (Supabase auth + Postgres: customers · clusters · scans · recommendations · savings_verified · actions ledger; kubeconfig connect; scheduled scans) → GitHub App replacing tokens → policy engine ($ threshold auto-PRs, weekly cap, namespace allowlist) → weekly digest ("found $X; 3 PRs open, 2 merged") → verified-savings ledger (30d baseline vs post-merge actual, verified at day 14, bill-level, honest "unverified" state) → billing.
**BILLING DECISION (one-time, by legal entity):** Indian entity → Merchant of Record (Paddle/Lemon Squeezy/Dodo — legal seller, EU VAT/US sales tax/GST + native USD recurring; ~5%+ fees beat self-managed tax below ~$15–20k MRR; Razorpay = gateway not MoR, international recurring needs extra setup — keep for India-domestic later). Stripe-country entity → Stripe. **Tiers:** Free (CLI forever + 1 hosted cluster) · Team $99 (3 clusters, auto-PRs, digest) · Growth $199 (10 clusters, ledger, policy). CLI never becomes a trial.
**PER-CUSTOMER CYCLE (the money loop in miniature):** connect cluster → first scan → first PR → merged? → verify savings → digest lands weekly → convert to paid → connected clusters grow → repeat.
**EXIT UP — GATE REVIEW (weeks 11–12, written + dated):** paying ≥1–3 · merged-PR rate healthy · verified savings on ≥3 real clusters · marketplace listings live (GitHub Marketplace; AWS Marketplace started) → Loop D.
**EXIT DOWN:** can't verify savings on 3 clusters by ~week 8 → trust failure → fix sizing before ANY other work; installs-but-no-merges → workflow failure → PR quality only; gate fails overall → Pivot Loop.

---

## LOOP D — EXPANSION LOOP (deepen per customer, month 3+)
**ENTRY:** gate passed. **CADENCE:** per existing customer, never cold.
**CYCLE:** offer incident mode free to an existing cost customer → Alertmanager webhook → pull pods/logs/events → correlate with deploy history → ranked hypotheses + confidence + evidence → human-approved bounded remediation → auto-postmortem → hypotheses accepted? → convert to premium → their accumulated cost-mode context deepens → next customer.
**WHY UPsell-ONLY:** Resolve AI ($150M), Cleric, and YC's Metoro own cold incident-PRs; your edge is weeks of per-cluster context they can't have on day one.
**EXIT UP:** the bigger-and-bigger ladder — cost tool → reliability layer → the GitOps action layer for cluster economics (every infra $ decision lands as a reviewed PR).
**EXIT DOWN:** existing customers won't try it even free → thesis on the moat is wrong → stay a cost tool, go wide. Still a real business.

---

## THE FOREVER LOOPS (run underneath once ignited — check monthly that each still spins)
- **Runtime loop:** observe → detect waste → dollarize → propose PR → human merges → verify on bill → learn → repeat. (Waste regrows weekly as new services ship with default requests — the reason this is a subscription.)
- **Growth loop:** CLI prints a dollar figure → pasted into Slack → budget owner sees it → some install hosted → stars/marketplace rank compound → more installs. The output IS the marketing.
- **Money loop:** verified savings → trust → more clusters connected → more context → premium tiers → two load-bearing systems → switching cost → retention.
- **Evolution loop:** usage friction observed → cluster repeated behavior → identify next feature → value-check → ship → measure → keep or kill.
**Monthly health check:** if any forever loop has stalled, that — not new features — is the week's work.

---

## PIVOT LOOP (failure-to-idea, entered ONLY via an exit-down)
**CYCLE:** 1. diagnose: product failure or distribution failure (never conflate) → 2. extract the strongest insight → 3. list what transfers (read layer, GitHub App, billing/MoR, packaging pipeline, OSS track record, FinOps+GitOps expertise — you restart ~60% tooled-up and more hireable; that's the downside floor) → 4. compare survivor ideas against the named fallback **Sync Gap Factory** (app-store intent distribution, fastest cash of everything researched) → 5. kill the weaker option in writing, dated → 6. enter that idea's Loop A. No sunk cost, no idea-shopping outside this loop.

---

# MONEY MATH (unchanged, no fudging)
Conservative: $149 avg × 34 customers, month 5–7. Base: $220 blended × ~23, month 4–6. Aggressive: $400 avg × ~13, month 3–4 only if Loop B ignites early. Cost until revenue: <$20/month.

# LOOP A, CYCLE 1 — YOUR NEXT 5 HOURS
1. `kind` + kube-prometheus-stack up; padded workloads deployed.
2. Install KRR, run it, save the output — the gate cycle starts tomorrow with evidence in hand.
3. Reserve the GitHub repo + name. Nothing else. The CLI is the marketing.

**Claude Code starter prompt (fires in slice 3, after the gate):**
```
Build the MVP of "kubeloop", a READ-ONLY Kubernetes CLI in Go (single binary).
Architecture: a `Recommender` interface so the engine is swappable; implement
[ADAPTER FROM GATE: krr --json ingest OR own PromQL, CPU≈P95, mem=max-7d+15%].
`kubeloop scan`: (1) current kubeconfig, read-only; inventory Deployments/
StatefulSets with per-container requests; handle multi-container; exclude
CronJobs/Jobs and <7d-history with printed reasons. (2) proposed values from
the Recommender. (3) dollarize via editable pricing.yaml (AWS/GCP/Azure
defaults) + node-type detection; waste=(current−proposed)×price×replicas×730h;
detect GKE Autopilot → label savings "immediate". (4) colorized ranked table +
total + --json + confidence column; floors IN CODE: CPU ≥ P99×1.2, mem ≥
max+buffer. Never write to the cluster. MIT. README with example output.
Build incrementally — inventory → recommender → pricing → ranking — and let
me test each step.
```

The machine is defined. Every loop has its entry, its cycle, and its two exits. Start Loop A, cycle 1.
