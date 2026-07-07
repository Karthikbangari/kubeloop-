# RULEBOOK — working rules for kubeloop

Operating doc for Claude Code, Codex, and any future coding agent working in this repo. Follow these rules and log every playground change below.

## The loop
1. I do all work in `playground/` first — never straight into the real tree.
2. Every playground change gets a **Log** entry here: what, why, which files.
3. **Codex reviews the code and approves.** Nothing leaves `playground/` until it does.
4. Approved work graduates out of `playground/` into the project proper; I note the graduation in the log.

## Loop cadence
- Claude Code runs one build/graduate iteration, then schedules the next ~25 minutes out — it works in bursts, not continuously.
- When an iteration stops, the ~25-minute gap is Codex's window: Codex checks the latest playground change and records its verdict in the log (see below).
- The next Claude Code iteration reads Codex's verdict first, then graduates what was approved and/or starts the next slice.
- The loop keeps running on this cadence until the user says `stop`.

## Automatic playground review

When Claude Code adds, pushes, or changes anything, Codex automatically checks `playground/` first:

1. Read the newest `RULEBOOK.md` log entry.
2. Inspect the changed playground files.
3. Run the relevant local checks when possible.
4. Record the Codex review result in this log.
5. Do not graduate code out of `playground/` until the review says it is ready.

## Project guardrails (from plan/MASTER-PLAN-LOOPED.md — don't violate)
- Read-only against clusters. The only write path is a human-reviewed PR.
- Safety floors live in code: CPU ≥ P99×1.2, mem ≥ max+buffer.
- Under-claim dollars — list prices are directional; the ranking is the point.
- No new-idea shopping, no scope creep. Build only the current slice.

## Log
Newest first. One entry per playground change.

### #60 — 2026-07-06 — build slice: kube-object → inventory parser (slice-26, read-layer)
- **What:** `kubeparse.Parse(json)` reads a serialized Deployment/StatefulSet (kube API / `kubectl get -o json`) → `Workload{Kind,Namespace,Name,Replicas,Containers,InitContainers}` with requests parsed via `quantityparse` (#59). Replicas default to 1; a malformed request quantity errors (not a silent 0); absent requests → 0. Output feeds `inventory.PodRequest`/`DetectRuntime` directly.
- **Why:** the kube side of the read-layer, offline-provable against captured JSON; only the live API LIST call remains cluster-gated. Ties together inventory + quantityparse into "real k8s object → numbers the scanner uses".
- **Files:** `playground/slice-26-kubeparse/{kubeparse.go,kubeparse_test.go}` (imports the sibling `slice-25-quantityparse`; both graduate together).
- **Verified:** `go vet ./...` clean; `go test ./...` green — full multi/init-container parse (PodRequest 2250m, jvm detected), replicas default, malformed-quantity error, absent-requests-zero.
- **On graduation:** `internal/inventory` (with quantityparse); the live reader wraps a kube client LIST around it.
- **Codex status:** ⬜ awaiting review.

### #59 — 2026-07-06 — build slice: k8s quantity parser (slice-25, read-layer)
- **What:** `quantityparse.CPU(s)` → millicores ("2000m"→2000, "1.5"→1500) and `quantityparse.Mem(s)` → bytes ("512Mi", "1Gi", "1G", plain bytes). Correct-or-error; the inverse of the shipped `internal/pr/quantity` formatter and the read-layer's bridge from real kube objects' request strings to the numbers the scanner uses.
- **Why:** reading current requests from a live/serialized Deployment needs quantity parsing; without it the read-layer can't compute waste from real manifests. **Reverses** the earlier "defer to apimachinery" note (architecture doc) — the project stayed lightweight (yaml.v3 only) and this is symmetric with the approved formatter; correct-or-error keeps it trust-safe.
- **Files:** `playground/slice-25-quantityparse/{quantityparse.go,quantityparse_test.go}`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — CPU milli/cores/fractional + bad forms error; memory Ki/Mi/Gi/decimal/plain + bad forms error; parses the formatter's canonical output.
- **On graduation:** `internal/inventory` (or a shared quantity pkg with the formatter); update the architecture doc's "apimachinery quantity parsing" note. The live kube reader uses it to fill `inventory.Container`.
- **Codex status:** ⬜ awaiting review.

### #58 — 2026-07-06 — build slice: Prometheus HTTP client (slice-24, read-layer)
- **What:** `promclient.Client.Query(ctx, promQL) (val, ok, err)` — issues a read-only GET to `/api/v1/query`, parses via `internal/readlayer/promusage`. Empty result → `ok=false` (missing, not error), same contract that flows a metrics gap into safety's exclusion. `New` trims trailing slash, accepts an injected `*http.Client`.
- **Why:** the tool↔Prometheus HTTP integration is the next real read-layer piece and is fully offline-provable with `httptest`. Deliberately does NOT construct PromQL — those strings need a live Prometheus to validate, so query building stays a separate slice; the caller passes the query.
- **Files:** `playground/slice-24-promclient/{promclient.go,promclient_test.go}`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — fetch+parse (asserts the query round-trips URL-decoded and hits `/api/v1/query`), empty=missing, non-200 errors, trailing-slash trim.
- **On graduation:** `internal/readlayer/promclient`; the live reader pairs it with validated PromQL + `promusage.AssembleUsage` → `rs.Usage`.
- **Codex status:** ⬜ awaiting review.

### #57 — 2026-07-06 — verify + coverage: `--json` carries caution/reasons (pinned)
- **What:** verified the `--json` schema is honest for machine consumers — it already includes the JVM `caution` (omitempty) and per-workload exclusion `reason`s (unlike the PR body before #56). No bug. But no test pinned the caution field, so a refactor could silently drop it; added `TestRun_JSONIncludesCaution` (JVM input → JSON `caution` mentions "JVM").
- **Why:** the JSON is a public contract; the safety caution must survive there for anyone automating off `--json`.
- **Files:** `cmd/kubeloop/main_test.go` (test only, no behavior change).
- **Verified:** new test passes; `go vet ./...` clean; `go test ./...` green.
- **Codex status:** ⬜ awaiting review.

### #56 — 2026-07-06 — bug-hunt + FIX: PR dropped the JVM (safety) caution
- **What (finding):** `kubeloop pr` on a JVM workload patches memory 2Gi→628Mi (a 3.3× cut) but the PR body had **no mention** of the JVM caution that scan shows ("memory is heap-configured, not usage-driven"). The reviewer approving the PR — the human the whole read-only design relies on — never saw the warning. `pr.Change`/`Request` carried `Confidence` but not the caution.
- **What (fix):** added `Caution` to `pr.Change` and `pr.Request`, rendered it as a prominent `> ⚠ **Caution:** …` blockquote in the PR body, and wired `cmd/kubeloop pr` to pass `row.Caution` (from `safety.Score`) through. Independent of #55's pending files (`compose.go`/`prepare.go`/`main.go` weren't in it).
- **Why:** the caution exists precisely to stop bad memory cuts; dropping it in the PR path defeats the safety design. Surfacing (not refusing) keeps the human in control while making the risk visible.
- **Files:** `internal/pr/{compose.go,compose_test.go,prepare.go}`, `cmd/kubeloop/main.go`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — new `TestBody_SurfacesCaution` (and blank caution renders nothing); live JVM `pr` now prints the `⚠ Caution` line.
- **Codex status:** ⬜ awaiting review.

### #55 — 2026-07-06 — GRADUATE #53/#54 → safety missing-signal exclusion (+ fixture sweep)
- **What:** Codex cleared #54; graduated the missing-signal guard. `safety.Assess` now takes `Usage` and excludes zero-CPU-signal (`P95=P99=0` → would propose 0m) and zero-memory-signal (`MaxMem=0` → would propose the bare buffer) workloads *before* the batch/history rules, with printed reasons. `scan.Scan` passes usage. Removed `playground/slice-23-nosignal/`.
- **Fixture sweep (Codex's flagged risk):** many fixtures/examples omitted `MaxMem` and would now wrongly exclude. Updated `safety_test.go`, `scan_test.go`, `cmd/kubeloop/main_test.go`, `examples/offline-input.json`, `examples/checkout-deployment.yaml`, and the README example so rankable workloads carry real CPU+memory and the batch/short-history ones are excluded for their *type*. Batch/short-history fixtures now have usage (excluded for the right reason). `checkout-api` uses a CPU-only request (realistic) so its memory isn't spuriously patched.
- **Files:** `internal/safety/{safety.go,safety_test.go}`, `internal/scan/{scan.go,scan_test.go}`, `cmd/kubeloop/main_test.go`, `examples/*`, `README.md`; deleted `playground/slice-23-nosignal/`.
- **Verified:** `go build/vet/test ./...` all green. Live: the two dangerous inputs now print clean exclusions ("no measured CPU/memory usage — can't size") instead of `0m`/`128Mi` at high confidence; README example reproduces ($200.34 across 3, 2 excluded); `pr` example consistently reduces CPU 2000→576m and memory 512→428Mi.
- **Codex status:** ⬜ awaiting review of the graduated layout.

### #54 — 2026-07-06 — SAFETY (extends #53): memory metrics gap is the same bug
- **What (finding):** the #53 CPU fix was incomplete. A memory gap (`MaxMem=0`) with current 2Gi is scanned as **proposed `128Mi`, confidence HIGH** — a 16× cut that would OOM-kill a workload whose real need is unknown. Same "metrics gap → confident dangerous recommendation" class as the CPU case.
- **What (fix):** reworked slice-23 (not yet graduated) from a CPU-only bool to `nosignal.MissingSignal(usage) (reason, bool)` — excludes on CPU gap (P95=P99=0) or memory gap (MaxMem=0), returning the specific printable reason.
- **Why:** both CPU and memory sizing collapse to a dangerous value when their signal is missing; the exclusion must cover both, not just CPU.
- **Files:** `playground/slice-23-nosignal/{nosignal.go,nosignal_test.go}` (reworked in place, pre-graduation).
- **Verified:** `go vet ./...` clean; `go test ./...` green — healthy kept; cpu-gap, mem-gap, both-gaps (CPU reason first) excluded; p99-only still sizable.
- **On graduation:** `safety.Assess(Meta, Usage)` returns `MissingSignal`'s reason before the other exclusions; `scan.Scan` passes usage; verify both `0m`-CPU and `128Mi`-mem dangerous proposals become clean exclusions.
- **Codex status:** ✅ reviewed as playground work. This is the right safety boundary: missing CPU and missing memory signals both produce dangerous, confident recommendations today, so they should be excluded before ranking/scoring. On graduation, sweep existing CPU-only fixtures/examples that currently omit `MaxMem` — either add realistic memory usage for rankable workloads or assert the new exclusion where a missing-memory gap is intentional.

### #53 — 2026-07-06 — bug-hunt + fix (SAFETY): exclude zero-CPU-signal workloads (slice-23)
- **What (finding):** a workload with `P95=P99=0` CPU usage (metrics gap / not running) is scanned as **proposed `0m` CPU, confidence HIGH, "$47.94/month"** — a dangerous "cut to zero" recommendation. Root cause: the CPU floor `max(P95, P99×1.2)` collapses to 0, and the burstiness check needs `P95>0` so it isn't downgraded. Memory is safe (absolute `+128Mi` floor); CPU has no absolute floor. This is exactly the "confident nonsense" the safety layer must block (the project's credibility guardrail).
- **What (fix):** `nosignal.HasNoCPUSignal(usage)` — true when both CPU percentiles are 0. On graduation, `safety.Assess` excludes these with a printed reason ("no measured CPU usage — can't size"), like CronJob/<7d exclusions.
- **Files:** `playground/slice-23-nosignal/{nosignal.go,nosignal_test.go}`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — both-zero true; normal / p95-only / p99-only / mem-present-cpu-zero cases.
- **On graduation:** change `safety.Assess(Meta)` → `Assess(Meta, Usage)`, add the exclusion, and update `scan.Scan` to pass usage.
- **Codex status:** superseded by #54 before graduation. The CPU-gap finding was real, but the final approved slice is the broader `MissingSignal` guard that also excludes missing memory metrics.

### #52 — 2026-07-06 — coverage: CLI-level test for multi-container refusal
- **What:** added `TestRun_PRRefusesMultiContainer` in `cmd/kubeloop` — runs `kubeloop pr` against a sidecar (app+sidecar) manifest and asserts it errors citing "containers". Confirms the #50 `pr.Prepare` guard surfaces end-to-end to the CLI, not just at the package level.
- **Why:** the CLI already had regression tests for not-rankable / ambiguous / no-reduction errors but not the multi-container guard; this closes that gap so the hardening can't silently regress at the command boundary.
- **Files:** `cmd/kubeloop/main_test.go` (test only, no behavior change).
- **Verified:** new test passes; `go vet ./...` clean; `go test ./...` green.
- **Codex status:** ✅ approved. The test drives the real `Run("pr", ...)` path with an app+sidecar manifest and proves the `pr.Prepare` guard surfaces through the CLI, so the multi-container refusal is pinned at the command boundary.

### #51 — 2026-07-06 — bug-hunt: patcher robust on StatefulSet + flow-style (added regression test)
- **What:** verified `kubeloop pr` on a StatefulSet with flow-style requests (`requests: {cpu: 2000m, memory: 1Gi}`) — kind matched, both values reduced, and the inline flow style was preserved. No bug. Existing tests only covered block-style Deployment (StatefulSet appeared only as a negative mismatch case), so I locked the verified behavior in with `TestPatch_StatefulSetFlowStyleRequests`.
- **Why:** turn manual verification into permanent regression protection rather than leave verified edge cases untested.
- **Files:** `internal/pr/patcher_test.go` (test only, no behavior change).
- **Verified:** new test passes; `go vet ./...` clean; `go test ./...` green.
- **Codex status:** ✅ approved. The test covers a useful existing behavior boundary: StatefulSet identity matching plus yaml.v3 flow-style scalar updates. No behavior change, just durable regression coverage.

### #50 — 2026-07-06 — GRADUATE slice-22 → internal/pr guard (wired into Prepare) + playground cleanup
- **What:** Codex cleared #49; graduated `ContainerCount`/`RequireSingleContainer` into `internal/pr` and wired the check inside `pr.Prepare` (after `FindSource`, before `Patch`) so every PR caller — CLI included — inherits the multi-container refusal, per Codex's note. Removed `playground/slice-22-guards/`. Also swept 8 empty leftover directories (slice-14…21) that graduations left behind (git ignores empty dirs, so they were invisible to status and never affected `go test`).
- **Why:** `Prepare` is the single choke point for PR construction; guarding there protects all callers. The empty-dir sweep keeps the playground honestly reflecting "nothing in flight".
- **Files:** `internal/pr/{guards.go,guards_test.go}`, `internal/pr/prepare.go`; deleted `playground/slice-22-guards/` and 8 empty slice dirs.
- **Verified:** `go vet ./...` clean; `go test ./...` green — guard unit tests plus `TestPrepare_RefusesMultiContainer` proving the choke-point refusal.
- **Codex status:** ✅ approved. The guard graduated into the right boundary: `Prepare` locates the matching source file first, refuses multi-container manifests before patching, and leaves single-container behavior unchanged. Playground cleanup is consistent with the rulebook state.

### #49 — 2026-07-06 — bug-hunt + fix: refuse multi-container PRs (slice-22)
- **What (finding):** for a 2-container pod (app 2000m + sidecar 500m = 2500m pod-level), `kubeloop pr` writes the pod-level proposal (576m) into only the `--container` and leaves the sidecar at 500m → patched pod requests **1076m**, but the PR body claims `2500m → 576m`. The PR overstates the reduction whenever a sidecar exists.
- **Root cause:** the scan model is pod-level; a pod-level proposal can't be split across containers without per-container usage, which the offline model doesn't carry.
- **What (fix):** `guards.RequireSingleContainer(manifest)` refuses when the pod has >1 container — honest "refuse rather than emit a misleading PR", matching the locator/rowselect posture. Lifts when per-container proposals exist (see architecture Known limitations).
- **Files:** `playground/slice-22-guards/{guards.go,guards_test.go}`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — count single/multi, refuse multi citing "2 containers", malformed errors.
- **On graduation:** `internal/pr`; `runPR` calls it on the located manifest before patching.
- **Codex status:** ✅ reviewed as playground work. The bug is real: the current PR path applies a pod-level proposal to a single named container, which overstates before/after savings for sidecar pods. The guard is the right conservative fix until per-container proposals exist. On graduation, wire it after source location and before patching, ideally inside `pr.Prepare`, so only the matched manifest is checked and all PR callers inherit the refusal.

### #48 — 2026-07-06 — bug-hunt + FIX: `kubeloop scan` subcommand was broken
- **What (finding):** `kubeloop scan --from-file X` failed with "--from-file required". The dispatch only special-cased `pr`; for `scan`, the word "scan" was left as a positional arg, so `flag.Parse` stopped before `--from-file` and never read it. Only the bare `kubeloop --from-file X` worked — but the README/plan show `kubeloop scan`.
- **What (fix):** `Run` now switches on `args[0]` for both `pr` and `scan` (stripping the subcommand), with bare invocation still defaulting to scan. Real, user-facing bug (the documented invocation didn't work).
- **Why:** the CLI must honor its own documented `scan` command; this was a regression introduced when the `pr` subcommand was added (#42).
- **Files:** `cmd/kubeloop/{main.go,main_test.go}`.
- **Verified:** `go vet ./...` clean; `go test ./cmd/kubeloop` green incl. new regression test asserting both `--from-file X` and `scan --from-file X` render the table; live `kubeloop scan --from-file examples/offline-input.json` now prints the ranked table.
- **Codex status:** ✅ approved. Dispatch now strips the explicit `scan` subcommand before flag parsing while preserving bare scan and `pr`; the regression test covers both documented scan forms. Codex verified `go test ./cmd/kubeloop`, `go test ./...`, `go vet ./...`, `go build -o bin/kubeloop ./cmd/kubeloop`, and the live `./bin/kubeloop scan --from-file examples/offline-input.json` table output.

### #47 — 2026-07-06 — bug-hunt: patcher edge cases (limits/quotes preserved) + documented baseline limit
- **What:** exercised `kubeloop pr` against a manifest with `limits` and quoted quantities. Confirmed correct: only `requests` change (2000m→576m), `limits` untouched, quote style preserved (`"2000m"`→`"576m"`). Found no code bug. Surfaced one design note: reduce-only compares scan.current vs proposal, not manifest.current — safe in the real flow, divergent only offline. Documented it in `docs/architecture.md` rather than building speculative reconciliation (YAGNI until the live read-layer makes scan.current == manifest).
- **Why:** verify the patcher on realistic manifests and record a known limitation for the next agent instead of over-engineering.
- **Files:** `docs/architecture.md` (added "Known limitations"). No code change.
- **Verified:** `go test ./internal/pr ./cmd/kubeloop` green; `go test ./...` green; `go vet ./...` clean.
- **Codex status:** ✅ approved. The documented limitation matches the current offline CLI: reduction selection is based on scan rows, while patching verifies manifest identity/container and edits only `resources.requests`. This is the right note to carry until the live read-layer/source mapping makes scan-vs-manifest baselines authoritative.

### #46 — 2026-07-06 — verification milestone: offline v0.1 complete + hardened
- **What:** end-to-end verification of the graduated PR hardening (#45), plus a full health check. `kubeloop pr` now leaves memory at `0Mi` (no misleading increase) with a CPU-only body/rollback, and refuses `workload "api" matches 2 namespaces — pass --namespace`. `make ci` green across all 12 packages; `go vet` clean; playground empty.
- **Why:** confirm the two bug-hunt fixes actually took effect and the whole tree is healthy — closes out the offline build.
- **State:** the offline v0.1 is feature-complete and hardened: scan (dollar-ranked, confidence, exclusions, realization, text/JSON, pricing overrides) + read-layer offline halves (inventory, promusage) + PR engine (locate/patch/compose/prepare) + `kubeloop pr` CLI. Remaining work is environment-gated: live kube/Prometheus clients (+apimachinery), Helm/Kustomize source mapping, and git/GitHub PR creation.
- **Codex status:** n/a (verification, no code change).

### #45 — 2026-07-06 — GRADUATE PR hardening: reduce-only + ambiguous row refusal
- **What:** Codex cleared #43 and #44; graduated `Reductions` and `FindRow` into `internal/pr`, removed `playground/slice-20-reduceonly/` and `playground/slice-21-rowselect/`, and wired `cmd/kubeloop pr` to use both. The CLI now patches only actual request reductions, errors on no-op PRs, and refuses ambiguous workload names unless `--namespace` disambiguates.
- **Why:** a savings PR must not silently raise a request, and the PR path must never guess which same-named workload to patch. Both fixes align `kubeloop pr` with the existing "refuse rather than guess" and "never over-claim savings" guardrails.
- **Files:** `internal/pr/{reduceonly.go,reduceonly_test.go,rowselect.go,rowselect_test.go}`, `cmd/kubeloop/{main.go,main_test.go}`, `playground/README.md`; deleted the two playground slices.
- **Verified:** `./bin/kubeloop pr --from-file examples/offline-input.json --manifest examples/checkout-deployment.yaml --namespace shop --workload checkout-api --container app --out /private/tmp/checkout-deployment.patched.yaml` now patches CPU `576m` while leaving memory `0Mi`; `go test ./internal/pr ./cmd/kubeloop` green; `go test ./...` green; `go vet ./...` clean; `go build -o bin/kubeloop ./cmd/kubeloop` green.
- **Codex status:** ✅ approved. The CLI now refuses ambiguous scan rows, refuses no-reduction PRs, and only patches resources that actually lower requests.

### #44 — 2026-07-06 — bug-hunt + fix: pr must refuse ambiguous workload (slice-21)
- **What (finding):** `runPR`'s `findRow` returns the *first* row matching `--workload`, so a name shared across namespaces with no `--namespace` silently targets (and patches) the wrong workload — the manifest locator already refuses this, but row selection didn't.
- **What (fix):** `rowselect.Find(rows, namespace, name)` returns the single match, erroring on no match or ambiguity (asks for `--namespace`). Same refuse-rather-than-guess rule as the locator, applied to the scan row.
- **Why:** consistency + safety — the whole PR path must not silently act on the wrong workload; better to ask than mis-patch.
- **Files:** `playground/slice-21-rowselect/{rowselect.go,rowselect_test.go}`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — unique name, ambiguous-without-namespace errors (the found bug), namespace disambiguates, no-match errors.
- **On graduation:** replace `findRow` in `cmd/kubeloop` with `rowselect.Find` (home under `internal/pr` or a cmd helper). Also, with #43's reduce-only, `runPR` should error when there are no reductions (a no-op PR is not worth opening).
- **Codex status:** ✅ reviewed as playground work. The selector matches the manifest locator's safety posture: no silent first-match behavior, and same-named workloads require `--namespace`. Graduated with #45.

### #43 — 2026-07-06 — bug-hunt + fix: savings PR must not raise a request (slice-20)
- **What (finding):** end-to-end `kubeloop pr` on the example produced a "save ~$32/month" PR that *raised* memory `0Mi → 128Mi` (safety floor lifting a below-floor current request). The dollars were CPU-only and correct, but patching an *increase* into a savings PR is misleading.
- **What (fix):** `reduceonly.Reductions(current, proposed)` returns a resource's proposed value only when it's strictly lower than current, else 0 ("leave unchanged"). A savings PR then only ever reduces requests; a below-floor request is a separate under-provisioning concern.
- **Why:** matches the never-negative-waste principle already in `MonthlyWaste` and the honesty guardrail — the PR realizes savings, it doesn't silently bump requests up.
- **Files:** `playground/slice-20-reduceonly/{reduceonly.go,reduceonly_test.go}`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — both-reduce, memory-increase-skipped (the found bug), no-change-when-not-lower.
- **On graduation:** fold into `internal/pr`; `runPR` in `cmd/kubeloop` should pass proposed values through `Reductions` so it patches/labels only reductions.
- **Codex status:** ✅ reviewed as playground work. The helper correctly mirrors `MonthlyWaste`'s never-negative behavior for PR patches: reductions only, no hidden request increases. Graduated with #45.

### #42 — 2026-07-06 — CLI: offline `kubeloop pr` preparation
- **What:** wired an offline `pr` subcommand: scan offline inputs, find a rankable workload, convert numeric current/proposed requests to Kubernetes quantity strings, run `internal/pr.Prepare`, write the patched manifest to `--out`, and print the PR title/body. Added `examples/checkout-deployment.yaml` and README usage.
- **Why:** with `internal/pr` and `internal/pr/quantity` approved, this connects the offline-proven PR path without pretending GitHub, git branches, Helm/Kustomize source mapping, or cluster writes exist.
- **Files:** `cmd/kubeloop/{main.go,main_test.go}`, `internal/pr/compose.go`, `README.md`, `examples/checkout-deployment.yaml`.
- **Verified:** `./bin/kubeloop pr --from-file examples/offline-input.json --manifest examples/checkout-deployment.yaml --namespace shop --workload checkout-api --container app --out /private/tmp/checkout-deployment.patched.yaml` writes CPU `576m` / memory `128Mi` and prints title/body; `go test ./internal/pr ./cmd/kubeloop` green; `go test ./...` green; `go vet ./...` clean; `go build -o bin/kubeloop ./cmd/kubeloop` green.
- **Codex status:** ✅ approved. The CLI path is explicitly offline: it prepares a patched manifest and reviewer text, errors for non-rankable workloads, and does not touch git, GitHub, or the cluster. Codex adjusted PR body wording from "opened" to "prepared" to match that behavior.

### #41 — 2026-07-06 — GRADUATE slice-19 → internal/pr/quantity
- **What:** Codex cleared #40; graduated numeric request formatters to `internal/pr/quantity`. Removed `playground/slice-19-quantity/`.
- **Why:** this is the final offline bridge between scan's numeric proposals and `internal/pr`'s Kubernetes quantity-string patcher. Keeping it under `internal/pr` makes its patching purpose explicit.
- **Files:** `internal/pr/quantity/{quantity.go,quantity_test.go}`; deleted `playground/slice-19-quantity/`; updated `playground/README.md`.
- **Verified:** `go test ./internal/pr/quantity` green; `go test ./...` green; `go vet ./...` clean; `go build ./cmd/kubeloop` green.
- **Codex status:** ✅ approved. The formatter is now the PR package's quantity bridge: exact millicore CPU strings and conservative round-up memory strings for manifest patches.

### #40 — 2026-07-06 — build slice: numeric→quantity formatter (slice-19)
- **What:** `quantity.CPU(millicores)` → `"492m"` (exact); `quantity.Mem(bytes)` → rounds **up** to whole MiB (`"428Mi"`), prefers `Gi` on whole-GiB. The bridge from scan's numeric proposals to the PR patcher's quantity strings.
- **Why:** the patcher needs k8s quantity strings; scan produces numbers. Rounding memory up guarantees a proposal never dips below the computed request. Two bugs caught in-loop and fixed: a duplicate map key in the test, and `Mem(0)` wrongly rendering `"0Gi"` (added a `mi > 0` guard on the Gi branch).
- **Files:** `playground/slice-19-quantity/{quantity.go,quantity_test.go}`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — CPU cases, memory round-up (`428Mi+1→429Mi`), Gi preference (`2Gi`, `1Gi`), non-whole-Gi stays Mi.
- **On graduation:** `internal/pr` (or a small `internal/quantity`); feeds the future `kubeloop pr` command that turns a scan proposal into a prepared PR.
- **Codex status:** ✅ reviewed as playground work. The formatter is small, deterministic, and conservative: CPU stays exact in millicores, memory rounds up to avoid under-provisioning, and `Mem(0)` correctly stays `0Mi`.

### #39 — 2026-07-06 — GRADUATE PR offline path → internal/pr
- **What:** Codex cleared #35-#38 and graduated the PR-engine offline path into `internal/pr`: raw-YAML source locator, safe request patcher, PR title/body composer, and `Prepare` integration that returns `{Path, Content, Title, Body}` without touching git/GitHub. Removed `playground/slice-15-patcher/`, `slice-16-prcompose/`, `slice-17-locator/`, and `slice-18-prprepare/`. Updated the architecture doc.
- **Why:** these four slices form one coherent offline-proven PR core. The remaining PR tail is genuinely environment-gated: Helm/Kustomize rendered-to-source mapping and GitHub PR creation need a target repo/tools/token.
- **Files:** `internal/pr/{patcher.go,patcher_test.go,compose.go,compose_test.go,locator.go,locator_test.go,prepare.go,prepare_test.go}`; deleted the four playground slices; updated `playground/README.md` and `docs/architecture.md`.
- **Verified:** `go test ./internal/pr` green; `go test ./...` green; `go vet ./...` clean; `go build ./cmd/kubeloop` green. `go list` import graph confirms `internal/pr` only imports stdlib + `yaml.v3`.
- **Codex status:** ✅ approved. The offline PR path is cohesive and conservative: raw-YAML source location refuses ambiguity, patching verifies identity/container before editing, PR text surfaces evidence/rollback/read-only caveats, and `Prepare` returns no partial result on errors.

### #38 — 2026-07-06 — build slice: PR-engine end-to-end offline prepare (slice-18)
- **What:** `prprepare.Prepare(Request)` wires the three PR-engine pieces — locate source file → patch its requests → compose title/body — into one offline step returning `{Path, Content(patched), Title, Body}`. Any step's error short-circuits (no partial PR). No git/GitHub yet.
- **Why:** integration proof that locator+patcher+composer compose correctly, without deepening the Codex queue with a dependent-but-separate concern. (This tick found #35/#36/#37 still awaiting review, so graduation was blocked — built the integration instead of a fifth unrelated slice.)
- **Files:** `playground/slice-18-prprepare/{prprepare.go,prprepare_test.go}` (imports the sibling playground packages; all four graduate together to `internal/pr`).
- **Verified:** `go vet ./...` clean; `go test ./...` green — end-to-end (right path, patched cpu 492m, title $131, body rollback with original values), locate-error and patch-error propagation.
- **Codex status:** ✅ reviewed as playground work. The integration proves the offline PR path composes and short-circuits on locator/patch errors. Graduated with #39.

### #37 — 2026-07-06 — build slice: PR-engine manifest locator (slice-17)
- **What:** `locator.FindSource(files, Ref)` — scans repo manifest files and returns the one defining the target workload (kind/name, optional namespace). Refuses on zero matches or ambiguity (>1) instead of guessing; skips non-YAML files. Assumes one workload per file (common raw-YAML layout).
- **Why:** third PR-engine piece — find the file before patching it. The raw-YAML path is the "solid" case in the plan; helm/kustomize rendered→source mapping is a later refinement needing those tools.
- **Files:** `playground/slice-17-locator/{locator.go,locator_test.go}`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — returns matching path, no-match errors, ambiguous (duplicate) errors, namespace disambiguates same-named workloads.
- **On graduation:** `internal/pr` with patcher + composer.
- **Codex status:** ✅ reviewed as playground work. The raw-YAML locator is intentionally conservative: it refuses no-match and ambiguity instead of guessing, and leaves Helm/Kustomize source mapping for a tool-backed slice. Graduated with #39.

### #36 — 2026-07-06 — build slice: PR title/body composer (slice-16)
- **What:** `prcompose.Title/Body(Change)` — PR title leads with `~$X/month`; body is Markdown with savings + realization, a before/after resource table, confidence, the evidence (safety floors, directional prices), a rollback note with the original values, and a read-only disclaimer ("nothing was applied to the cluster"). Omits any resource left unchanged. Pure, dep-free.
- **Why:** second PR-engine piece; the reviewer-facing text is offline-testable and where the honesty guardrails must show up (no auto-apply claim, directional dollars, rollback path).
- **Files:** `playground/slice-16-prcompose/{prcompose.go,prcompose_test.go}`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — title has name+$131, body has evidence/rollback/read-only/table/confidence, CPU-only change omits the memory row and rollback mention.
- **On graduation:** `internal/pr` (with the patcher); the CLI's future `pr` command wires locator → patcher → prcompose → GitHub.
- **Codex status:** ✅ reviewed as playground work. The composer puts savings, realization, evidence, rollback, and read-only caveats in the reviewer path. On graduation Codex tightened "unchanged" handling so equal current/proposed values are omitted, not only empty fields.

### #35 — 2026-07-06 — build slice: PR-engine minimal-diff patcher (slice-15) + yaml.v3 dep
- **What:** started the PR engine (user picked this direction). Added `gopkg.in/yaml.v3 v3.0.1` (first dependency — authorized) and built `patcher.Patch(doc, Target, cpu, mem)`: verifies workload identity (kind/name/namespace), finds the target container, and updates only its `resources.requests` via `yaml.Node` — comments and other fields survive. Errors (never blind-guesses) on identity mismatch, missing container, or absent `resources.requests`.
- **Why:** the patcher is the trust-critical, offline-testable core of the PR engine; the manifest locator (`helm template`/`kustomize build` → source path) and PR composer are separate slices that need a repo/tools.
- **Files:** `playground/slice-15-patcher/{patcher.go,patcher_test.go}`; `go.mod`/`go.sum` (yaml.v3).
- **Verified:** `go vet ./...` clean; `go test ./...` green — updates+preserves (comment/replicas/image survive, old values gone), cpu-only leaves mem, identity/namespace mismatch errors, missing container/requests errors. Live demo confirmed the structure-preserving rewrite.
- **Honest limit:** yaml.Node re-encoding is structure/comment-preserving, not byte-minimal (normalizes indent to 2-space and comment spacing). Assumes a single-document 2-space manifest. Noted in the package doc.
- **Codex status:** ✅ reviewed as playground work. The patcher verifies workload identity and target container before editing, preserves comments/untouched fields through `yaml.Node`, and errors rather than blind-patching ambiguous manifests. Graduated with #39.

### #34 — 2026-07-06 — docs: architecture map of the 10 packages
- **What:** added `docs/architecture.md` — dependency diagram + per-package responsibility table + design rules + an explicit "not built yet" section (live clients, PR engine, hosted). Generated from the real `go list` import graph, not guessed.
- **Why:** the repo is heading public with 10 packages; a layering map helps contributors and Codex navigate. Documents what exists — no trust-critical guessing (unlike blind PromQL, which I'm deliberately not writing).
- **Files:** `docs/architecture.md`.
- **Verified:** dependency edges match `go list -f '{{.Imports}}'` output (rightsizing/labels/savings are leaves; scan orchestrates; inventory never imports scan). Doc-only, no code touched.
- **Codex status:** ✅ approved with #39 updates. The architecture map matched the real package graph at review time; Codex updated it during PR-engine graduation so `internal/pr` is now represented and the remaining PR tail is accurately scoped.

### #33 — 2026-07-06 — rule: document loop cadence (user request)
- **What:** added a "Loop cadence" section — Claude Code runs one iteration then schedules the next ~25 min out; the gap is Codex's review window; the next iteration reads Codex's verdict, graduates the approved, and starts the next slice; continues until the user says `stop`.
- **Why:** user asked to write the actual operating cadence into the rulebook so the Claude↔Codex handoff timing is explicit.
- **Files:** `RULEBOOK.md`.

### #32 — 2026-07-06 — GRADUATE slice-14 → internal/readlayer/promusage
- **What:** Codex cleared #31; graduated the Prometheus instant-response parser into `internal/readlayer/promusage`, keeping it under the read-layer namespace while leaving live HTTP and query construction out of scope. Removed `playground/slice-14-promusage/`.
- **Why:** usage parsing is the second offline-provable read-layer half, alongside inventory/request assembly. Putting it under `readlayer` keeps the future Prometheus client close without mixing it into the pure rightsizing core.
- **Files:** `internal/readlayer/promusage/{promusage.go,promusage_test.go}`; deleted `playground/slice-14-promusage/`; updated `playground/README.md`.
- **Verified:** `go test ./internal/readlayer/promusage` green; `go test ./...` green; `go vet ./...` clean; `go build ./cmd/kubeloop` green. During graduation, Codex fixed a copied test assertion format typo in the rounding table (`want` was not passed to `Errorf`).
- **Codex status:** ✅ approved. The package stays scoped to offline-verifiable Prometheus response parsing and unit conversion, correctly treats empty results as missing rather than error, and leaves live HTTP/PromQL validation for a real Prometheus-backed slice.

### #31 — 2026-07-06 — build slice: Prometheus usage parsing (slice-14, read-layer usage half)
- **What:** `promusage` — `Scalar([]byte)` parses a Prometheus `/api/v1/query` instant response to the first sample's value (empty result = missing, not error), `CoresToMilli` rounds cores→millicores, `AssembleUsage` builds `rs.Usage` (CPU cores→milli, mem bytes). Dep-free (encoding/json).
- **Why:** the usage half of the read-layer, provable offline against captured Prometheus JSON. Response parsing is trust-critical (wrong parse → wrong recommendation), so it's the part built and tested here.
- **Scope note (ponytail):** the PromQL query *strings* are deliberately NOT included — they can't be validated without a live Prometheus, so shipping guessed queries as "done" would be dishonest. That's a separate slice against a real Prom.
- **Files:** `playground/slice-14-promusage/{promusage.go,promusage_test.go}`.
- **Verified:** `go vet ./...` clean; `go test ./playground/slice-14-promusage` green — success, empty=missing, non-success status errors, malformed/non-numeric errors, rounding table, unit assembly.
- **On graduation:** `internal/promusage` (or fold into `internal/readlayer`); the live reader wires HTTP + queries around it.
- **Codex status:** ✅ reviewed as playground work. The parser correctly distinguishes missing data from malformed/error responses, converts CPU cores to millicores, and avoids unvalidated PromQL strings. Good to graduate under the read-layer namespace.

### #30 — 2026-07-06 — packaging: GoReleaser config and release workflow
- **What:** added `.goreleaser.yaml` for cross-platform `kubeloop` archives/checksums, a tag-triggered `.github/workflows/release.yml` using `goreleaser/goreleaser-action@v6`, and local `make release-check` / `make release-snapshot` targets for validation and unpublished release builds.
- **Why:** packaging needs a real release path after CI/README. This stays conservative: no Homebrew tap, signing, Docker image, or package-manager publishing until distribution credentials and ownership are decided.
- **Files:** `.goreleaser.yaml`, `.github/workflows/release.yml`, `Makefile`, `.gitignore`.
- **Verified:** `go run github.com/goreleaser/goreleaser/v2@v2.17.0 check` validates the config; `go run github.com/goreleaser/goreleaser/v2@v2.17.0 release --snapshot --clean` builds linux/darwin/windows archives for amd64/arm64 plus `checksums.txt`; `make help` lists release targets; `make ci` green; `make build` green. Local validation required network approval because GoReleaser v2.17.0 requires Go 1.26.4 for the tool itself.
- **Codex status:** ✅ approved. The release setup is intentionally narrow and functional: tag pushes publish through GitHub Releases, local snapshots work, generated `dist/` and validation caches are ignored, and deprecated GoReleaser keys were fixed after `check` flagged them.

### #29 — 2026-07-06 — packaging: GitHub Actions CI workflow
- **What:** added `.github/workflows/ci.yml` — on push-to-main and every PR, checks out, sets up Go 1.23, and runs `make ci` (vet + tests) then `make build`. Least-privilege `permissions: contents: read`.
- **Why:** the ignition plan wants CI green before going public; reuses the #24 Makefile targets so local and CI run the same commands. No CI badge added yet (would 404 until the first run — ponytail: don't ship a fake badge).
- **Files:** `.github/workflows/ci.yml` (infra, not playground code — logged for Codex review).
- **Verified:** `go-version 1.23` matches `go.mod`; `make ci` and `make build` both pass locally with the workspace toolchain. (Couldn't lint the YAML here — no `pyyaml` — but it's minimal standard workflow syntax.)
- **Codex status:** ✅ approved. The workflow is least-privilege, uses the repo Makefile as the shared CI contract, and local `make ci` plus `make build` pass with the workspace Go toolchain.

### #28 — 2026-07-06 — docs: finalize README example with real CLI output
- **What:** removed the SPEC DRAFT banner; added a reproducible `examples/offline-input.json`; replaced the mocked cluster/PR demo with output captured from the current `./bin/kubeloop --from-file examples/offline-input.json --cloud aws`; updated build/usage docs to the actual v0.1 flags; corrected the roadmap so live read-layer and PR engine are still pending.
- **Why:** the README should describe the real offline scan path that exists today, not future product behavior. This closes the README TODO to replace example output with real tool output.
- **Files:** `README.md`, `examples/offline-input.json`.
- **Verified:** `./bin/kubeloop --from-file examples/offline-input.json --cloud aws` matches the README example; `./bin/kubeloop --from-file examples/offline-input.json --cloud aws --json` emits the stable JSON schema; `go test ./...` green; `go vet ./...` clean; `go build -o bin/kubeloop ./cmd/kubeloop` green; `rg` found no leftover `SPEC DRAFT`, fake install, or `kubeloop scan` examples in `README.md`.
- **Codex status:** ✅ approved. The README now reflects the real v0.1 offline CLI instead of aspirational live scan/PR behavior, and the included fixture makes the example reproducible.

### #27 — 2026-07-06 — GRADUATE slice-13 → internal/readlayer
- **What:** Codex cleared #26; graduated the offline assembly to a new `internal/readlayer` package (renamed `assembly`→`readlayer`), NOT into `internal/inventory`. Per Codex's coupling note: `inventory` stays pure (no `scan` import); the glue that maps inventory → `scan.Input` lives in `readlayer`, which is also the future home of the live cluster reader. Removed `playground/slice-13-offline-assembly/`.
- **Why:** keep the dependency direction clean — inventory is a leaf, readlayer composes inventory+scan.
- **Files:** `internal/readlayer/{readlayer.go,readlayer_test.go}`; deleted `playground/slice-13-offline-assembly/`; updated `playground/README.md`.
- **Verified:** `go test ./internal/readlayer` green; `go vet ./...` clean; `go test ./...` green (all 9 pkgs) — assembly maps requests/runtime/usage (init-peak CPU 900, regular-sum mem 768Mi, jvm detected) and feeds `scan.Scan` end-to-end (ranked api / excluded nightly).
- **Codex status:** ✅ approved. The graduated layout keeps `inventory` as a leaf and puts the scan glue in `readlayer`, which is the right future home for live collection. Codex fixed the stale package comment and playground status while reviewing.

### #26 — 2026-07-06 — build slice: offline inventory assembly (slice-13)
- **What:** `assembly.ToScanInputs([]Workload) []scan.Input` behind a fake offline read-layer model. It takes workload metadata, containers, init containers, and usage; computes effective pod requests with `inventory.PodRequest`; detects runtime with `inventory.DetectRuntime`; and emits the `scan.Input` shape the existing scanner already consumes.
- **Why:** next read-layer progress without requiring a live cluster, kubeconfig, Prometheus, or apimachinery. The future live Kubernetes reader can feed this same model after quantity parsing at the apimachinery boundary.
- **Files:** `playground/slice-13-offline-assembly/{README.md,assembly.go,assembly_test.go}`, `playground/README.md`.
- **Verified:** `go test ./playground/slice-13-offline-assembly` green; `go test ./...` green; `go vet ./...` clean; `go build ./cmd/kubeloop` green. In this sandbox, commands used `PATH=/Users/karthikbangari/.local/go/bin:$PATH GOCACHE=/Users/karthikbangari/kubeloop/.gocache`.
- **Codex status:** ✅ reviewed as playground work. The fake model is the right offline boundary for the next read-layer step, reuses the graduated `inventory` primitives instead of duplicating request/runtime logic, and feeds `scan.Scan` directly without introducing cluster access or new dependencies. On graduation, decide whether this belongs in `internal/inventory` or a small adjacent read-layer package to avoid over-coupling inventory to scan.

### #25 — 2026-07-06 — Codex review: packaging Makefile (#24)
- **What:** reviewed #24's Makefile packaging targets and portability fix.
- **Why:** the Makefile is now the project entrypoint for build/test/vet/ci, so it needs to be trusted before the next slice uses it.
- **Files:** `Makefile`, `.gitignore` (read only during review).
- **Verified:** `make help` lists the expected targets. This sandbox does not have `go` on `PATH` and cannot write the default Go cache under `~/Library/Caches`, so verification used `PATH=/Users/karthikbangari/.local/go/bin:$PATH GOCACHE=/Users/karthikbangari/kubeloop/.gocache`: `make ci` green and `make build` green.
- **Codex status:** ✅ approved. The targets are conventional, the GNU Make 3.81 portability issue is resolved by tab-indented recipes, and build/test/vet all pass with the local Go binary and workspace cache.

### #24 — 2026-07-06 — packaging: Makefile build/test/ci targets (+ portability fix)
- **What:** added `build`, `test`, `vet`, `ci` (vet+test), and `clean` targets so contributors/CI have one-command build and test. While doing so, found the Makefile didn't run at all on macOS's default GNU Make **3.81**: it used `.RECIPEPREFIX := >`, which needs make ≥3.82 (`missing separator` error). Converted all recipes to standard TAB indentation and dropped `.RECIPEPREFIX`, so it works on every make version. Added `.PHONY`.
- **Why:** packaging slice needs a real build/test entrypoint; a Makefile that errors on the default toolchain is a pre-existing bug, not new scope. `bin/` is already gitignored.
- **Files:** `Makefile` (infra, not playground code — logged for Codex review).
- **Verified:** on GNU Make 3.81 — `make help` lists targets, `make ci` runs vet+all-tests green, `make build` produces `bin/kubeloop` that runs (prints the expected `--from-file` message), `make clean` removes `bin/`.
- **Codex status:** ✅ approved in #25.

### #23 — 2026-07-06 — GRADUATE slice-12 → internal/inventory (merged Container type)
- **What:** Codex cleared #22; folded `DetectRuntime` into `internal/inventory` and merged the two `Container` structs into one (`Image`, `Command`, `CPU`, `Mem`) per the review question — a container has both identity and requests, so `PodRequest` and `DetectRuntime` share it. Removed `playground/slice-12-runtime/`.
- **Why:** one read-layer struct is simpler than two parallel ones; the merge doesn't change either function's behavior (extra fields default to zero for `PodRequest` callers).
- **Files:** `internal/inventory/{inventory.go,inventory_test.go}`; deleted `playground/slice-12-runtime/`.
- **Verified:** `go vet ./...` clean; `go test ./...` green (all 8 pkgs) — PodRequest tests unchanged, DetectRuntime image/command/any-container/no-false-positive.
- **Codex status:** ✅ approved. The merged `Container` type is a better read-layer boundary, runtime detection stays conservative, and `go test ./...`, `go vet ./...`, and `go build ./cmd/kubeloop` pass.

### #22 — 2026-07-06 — build slice: JVM runtime detection (slice-12)
- **What:** `runtimehint.DetectRuntime([]Container) string` — returns "jvm" if any container's image matches a curated JVM base-image list or its command runs `java`, else "". Feeds `safety.Meta.Runtime` so the JVM memory caution fires automatically.
- **Why:** the read-layer needs to populate the runtime hint that `safety.Score` already consumes; a miss just means no caution (never a wrong number), and `node`/`javascript` explicitly don't false-positive.
- **Files:** `playground/slice-12-runtime/{runtime.go,runtime_test.go}`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — JVM image, `java` command, any-container-counts, non-JVM empty, node-not-java.
- **On graduation:** fold into `internal/inventory` (same read-layer concern) — decide whether to merge the `Container` type with inventory's.
- **Codex status:** ✅ reviewed as playground work. `go test ./...`, `go vet ./...`, and `go build ./cmd/kubeloop` pass with workspace-local `GOCACHE`. The heuristic is suitably conservative for driving a caution flag: false negatives only omit a caveat, while obvious non-JVM cases avoid false positives.

### #21 — 2026-07-06 — GRADUATE slice-11 → internal/inventory
- **What:** Codex cleared #20; moved `PodRequest` to `internal/inventory` unchanged. Removed `playground/slice-11-inventory/`.
- **Why:** approved work leaves the playground (rule 4).
- **Files:** `internal/inventory/{inventory.go,inventory_test.go}`; deleted `playground/slice-11-inventory/`.
- **Verified:** `go vet ./internal/inventory` clean; `go test ./internal/inventory` green.
- **Codex status:** ✅ approved. The graduated layout preserves the reviewed effective-request behavior, keeps live Kubernetes parsing out of the pure core, and full test/vet/build verification passes.

### #20 — 2026-07-06 — build slice: effective pod request (slice-11 inventory, read-layer core)
- **What:** `inventory.PodRequest(regular, init []Container) rs.Resources` — the Kubernetes effective-request rule per resource: `max(sum(regular), max(init))`. First piece of the read-layer.
- **Why:** the read-layer's trickiest correctness bit (init containers run sequentially → peak, not sum). Deliberately dep-free: quantity-string parsing ("512Mi") is left to the apimachinery boundary in the live client, so this stays offline-testable and isn't a throwaway reimplementation of `resource.Quantity`.
- **Files:** `playground/slice-11-inventory/{inventory.go,inventory_test.go}`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — regular-sum, init-peak-wins, regular-sum-wins, per-resource-independent (init wins CPU / regular wins mem), empty.
- **On graduation:** `internal/inventory`; the live read-layer feeds it numeric requests parsed via apimachinery, plus Kind/Namespace/Name/Replicas → `scan.Input`.
- **Codex status:** ✅ reviewed as playground work. `go test ./...`, `go vet ./...`, and `go build ./cmd/kubeloop` pass with workspace-local `GOCACHE`. The pure function correctly models Kubernetes effective pod requests without pulling live-client dependencies into the offline core.

### #19 — 2026-07-06 — GRADUATE slice-10 → internal/labels (+ wired into reporting & scan)
- **What:** Codex cleared #18; graduated to `internal/labels`. Replaced `reporting.workloadLabels` with a thin `rowLabels` that delegates to `labels.Qualify`, and applied the same helper to `scan.Render`'s Excluded section so duplicate excluded names get `namespace/name` like ranked rows. Removed `playground/slice-10-labels/`.
- **Why:** closes Codex's #11 polish note; the collision logic is now single-sourced across both lists instead of living only in reporting.
- **Files:** `internal/labels/*`, `internal/reporting/table.go`, `internal/scan/{scan.go,scan_test.go}`; deleted `playground/slice-10-labels/`.
- **Verified:** `go vet ./...` clean; `go test ./...` green (all 7 pkgs) — existing table/collision tests still pass, new scan test asserts `team-a/batch-job` vs `team-b/batch-job` in the excluded list.
- **Codex status:** ✅ approved. The graduated layout keeps labels single-sourced, reporting and scan both use the same collision logic, and full test/vet/build verification passes.

### #18 — 2026-07-06 — build slice: shared collision-aware labels (slice-10)
- **What:** `labels.Qualify([]Item) []string` — bare name normally, `namespace/name` only when a name collides across namespaces, empty namespace always bare. Pure, dep-free. Extracts the collision logic currently living unexported in `reporting.workloadLabels` so the ranked table AND the excluded list can share it.
- **Why:** Codex's #11 polish note — excluded workloads with duplicate names should disambiguate like ranked rows do. Building the shared helper (not copying the logic) keeps the two lists consistent.
- **Files:** `playground/slice-10-labels/{labels.go,labels_test.go}`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — collisions qualified / unique bare / empty-namespace bare / empty input.
- **On graduation:** put at `internal/labels`; refactor `reporting.workloadLabels` to call it, and use it in `scan.Render` for the excluded section.
- **Codex status:** ✅ reviewed as playground work. `go test ./...`, `go vet ./...`, and `go build ./cmd/kubeloop` pass with workspace-local `GOCACHE`. The helper preserves the existing ranked-table behavior and is the right shared primitive for excluded labels.

### #17 — 2026-07-06 — GRADUATE slice-09 → reporting.LoadPrice (+ --pricing-file, docs aligned)
- **What:** Codex cleared #16 with "don't leave impl and docs split." Committed to JSON (zero-dep, consistent with `--from-file`): folded the loader into `internal/reporting` as `LoadPrice`/`PriceFile`/`PriceRate` (reusing the existing `gib`/`milliPerVCPU` constants, no duplication), added `--pricing-file` to `cmd/kubeloop`, and updated the README from `pricing.yaml` → `pricing.json` (`--pricing-file`). Removed `playground/slice-09-pricing/`.
- **Why:** ponytail — a YAML parser is an avoidable dependency for one small config; JSON matches the tool's existing input format. Docs now match the implementation.
- **Files:** `internal/reporting/{pricing.go,pricing_test.go}`, `cmd/kubeloop/{main.go,main_test.go}`, `README.md`; deleted `playground/slice-09-pricing/`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — reporting loader tests (default/partial/unlisted/errors) plus a CLI test proving a higher `--pricing-file` CPU rate raises reported waste vs default.
- **Codex status:** ✅ approved. `pricing.json` resolves the docs/implementation split, the loader is in the right reporting layer, `--pricing-file` is wired through the CLI, and `go test ./...`, `go vet ./...`, and `go build ./cmd/kubeloop` pass.

### #16 — 2026-07-06 — build slice: editable pricing overrides (slice-09)
- **What:** `pricing.Load(cloud, file)` layers a user override file over the built-in cloud defaults. Overrides are per-field (readable per-vCPU-h / per-GB-h keyed by cloud); a zero/omitted field keeps the default, an unlisted cloud keeps its default. No file → pure default. Dep-free (encoding/json).
- **Why:** the plan/README promise "editable pricing" so teams can use their negotiated rates; delivered without pulling in a YAML dependency.
- **Files:** `playground/slice-09-pricing/{pricing.go,pricing_test.go}`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — default passthrough, CPU-only override keeps mem default, unlisted cloud keeps default, bad/missing file errors.
- **Open for Codex:** README says `pricing.yaml` but this is JSON (no-dep choice) — decide: add `yaml.v3` for real YAML, or update the doc wording. On graduation, fold into `internal/reporting` and add a `--pricing-file` flag to `cmd/kubeloop`.
- **Codex status:** ✅ reviewed as playground work. `go test ./...`, `go vet ./...`, and `go build ./cmd/kubeloop` pass with workspace-local `GOCACHE`. Before graduation, either support actual YAML to match the README or deliberately update the docs/flag naming to `pricing.json`; do not leave the implementation and docs split.

### #15 — 2026-07-06 — GRADUATE slice-07 → cmd/kubeloop (+ explicit JSON schema)
- **What:** Codex cleared #12; graduated the CLI to `cmd/kubeloop`. Addressed the review note with an explicit public wire type (`jsonReport`/`jsonWorkload`/`jsonExcluded`, camelCase keys) built by `toJSON`, so `--json` no longer marshals internal structs directly and internal refactors can't silently break the contract. Flags: `--json`, `--cloud`, `--from-file`, `--per-request`. Removed `playground/slice-07-cli/` — playground is now empty.
- **Why:** first real binary in the tree; the decoupled schema is the public API, kept stable on purpose.
- **Files:** `cmd/kubeloop/{main.go,json.go,main_test.go}`; deleted `playground/slice-07-cli/`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — text (table+realization+excluded), JSON unmarshalled into the public type (proposed CPU 576 hand-checked, positive total, realization present), `--per-request` immediacy, missing-flag error. `go build ./cmd/kubeloop` produces a working binary.
- **Codex status:** ✅ approved. The CLI moved to the right `cmd/kubeloop` home, the JSON contract is explicit instead of leaking internal structs, and test/vet/build pass.

### #14 — 2026-07-06 — GRADUATE slice-08 → internal/savings (+ wired into scan)
- **What:** Codex cleared #13; graduated to `internal/savings` (kept separate from `reporting` — distinct honesty concern) and wired it in per the review note. `savings` exposes `Realization(mode)` (clause) + `Headline(total,mode)`. `scan.Report` gained a `Mode`, `scan.Scan` takes it, and `scan.Render` prints the realization line under the total. CLI got a `--per-request` flag (default node-based, the conservative wording). Removed `playground/slice-08-savings/`.
- **Why:** the label is only useful if users see it; unknown/default mode uses the non-over-promising node-consolidation wording, honoring the under-claim guardrail.
- **Files:** `internal/savings/*`, `internal/scan/{scan.go,scan_test.go}`, `playground/slice-07-cli/main.go`; deleted `playground/slice-08-savings/`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — savings realization/headline tests, scan render asserts the "consolidate" clause; live CLI shows immediate vs consolidation for `--per-request` vs default.
- **Note:** `playground/slice-07-cli` (CLI, approved #12) now depends on the new `savings` flag; still awaiting graduation to `cmd/kubeloop` with an explicit JSON type.
- **Codex status:** ✅ approved. Keeping savings separate is clean, the realization wording is now visible in scan/CLI output, default node-based wording stays conservative, and test/vet/build pass.

### #13 — 2026-07-06 — Codex review: unlogged savings slice (slice-08)
- **What:** found `playground/slice-08-savings/{savings.go,savings_test.go}` during automatic playground review. It labels when estimated dollars are realized: per-request billing says immediate; node-based or unknown modes say savings realize when nodes consolidate.
- **Why:** this is the bill-honesty guardrail from the plan in small, testable logic. Process note: this slice was present without a matching rulebook entry, so Codex added this review entry to restore the log.
- **Files:** `playground/slice-08-savings/{savings.go,savings_test.go}`.
- **Verified:** `go test ./...` and `go vet ./...` pass from the root module with workspace-local `GOCACHE`.
- **Codex status:** ✅ reviewed as playground work. Before graduation, decide final home (`internal/savings` or fold into `internal/reporting`) and wire the headline into report rendering so the honesty label is actually visible to users.

### #12 — 2026-07-06 — build slice: kubeloop CLI (slice-07)
- **What:** `package main` for the `kubeloop` binary. `Run(args, out)` (split from `main` for testability) parses stdlib `flag`s — `--json`, `--cloud`, `--from-file` — loads workloads from a JSON file, runs `scan.Scan`, and writes the text report or indented JSON. Read-only always; no cluster writes.
- **Why:** first runnable binary. The live read-layer (kubeconfig+Prometheus) needs a cluster that's unavailable here, so `--from-file` is the offline input source it will later replace; keeps the CLI real and testable now.
- **Files:** `playground/slice-07-cli/{main.go,main_test.go}`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — text output has table+CONF+Excluded, `--json` is valid JSON (1 row + 1 excluded, total>0), missing `--from-file` errors. Live `go run` produced both the ranked table and JSON.
- **Open for Codex:** on graduation move to `cmd/kubeloop`; decide the JSON schema is stable enough to commit to (currently marshals internal structs directly).
- **Codex status:** ✅ reviewed as playground work. `go test ./...` and `go vet ./...` pass from the root module with workspace-local `GOCACHE`. Before graduation, move it to `cmd/kubeloop` and introduce an explicit JSON output type instead of marshaling internal structs directly.

### #11 — 2026-07-06 — GRADUATE slice-06 → internal/scan (+ single-source render)
- **What:** Codex cleared #10; graduated to `internal/scan` and folded rendering back through `internal/reporting` per review. `reporting.Row` gained optional `Confidence`/`Caution` strings; `reporting.Render` now shows a CONF column only when set and prints cautions under the table — so resource formatting and namespace-collision labels are single-sourced. `scan.Render` delegates the table to `reporting` and only appends the Excluded section; the duplicated `resStr`/`memStr` are gone. Removed `playground/slice-06-scan/`.
- **Why:** kill the duplication Codex flagged; `reporting` stays decoupled from `safety` by taking plain confidence strings, not `safety` types.
- **Files:** `internal/scan/{scan.go,scan_test.go}`, `internal/reporting/{table.go,table_test.go}`; deleted `playground/slice-06-scan/`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — scan exclude/rank/score/render tests, plus a new reporting test that CONF/caution appear only when set (blank stays a plain table, backward-compatible).
- **Codex status:** ✅ approved. The graduated layout satisfies the single-source rendering review note, keeps reporting decoupled from safety, and root-module tests/vet pass. Future polish: consider namespace-qualified labels for excluded workloads too, so duplicate excluded names are unambiguous like ranked rows.

### #10 — 2026-07-06 — build slice: end-to-end offline scan (slice-06)
- **What:** `scan` package ties all three graduated packages into one path — `Scan(inputs, rec, price)` does assess→exclude→rank→score, returning a `Report` (ranked rows + parallel confidence + excluded-with-reasons + total). `Render` adds a CONF column, JVM cautions, and an Excluded section. Ran a throwaway demo (removed) — output matches the README vision.
- **Why:** `/loop` → first end-to-end integration; fully offline (takes injected workloads; the read-layer that produces them from the cluster is still blocked here — no live cluster).
- **Files:** `playground/slice-06-scan/{scan.go,scan_test.go}`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — 3 tests: exclude+rank+score (2 excluded w/ reasons, order, $530.27 total, high/high conf), JVM caution (spiky+jvm → low), render sections. Live demo rendered the ranked table + cautions + exclusions correctly.
- **Open for Codex:** `resStr`/`memStr` are duplicated from `internal/reporting` (unexported there) — fold the two renderers together on graduation; and decide whether `scan` lives at `internal/scan` feeding a future `cmd/kubeloop`.
- **Codex status:** ✅ reviewed. `go test ./...` and `go vet ./...` pass from the root module with workspace-local `GOCACHE`. The slice is acceptable playground work; before graduation, put it in `internal/scan` and fold rendering back through `internal/reporting` so namespace collision labels and resource formatting stay single-sourced.

### #9 — 2026-07-06 — GRADUATE slice-05 → internal/safety
- **What:** Codex cleared #8; graduated to `internal/safety` (kept separate from `rightsizing` per review). Addressed the casing-drift note by making `Assess`/`Score` compare Kind/Runtime with `strings.EqualFold`, so `cronjob`/`CronJob` and `jvm`/`JVM` all hit their rules — defense in depth until a read-layer normalizes at its boundary. Removed `playground/slice-05-safety/`.
- **Why:** approved work leaves the playground (rule 4); case-insensitive matching prevents a silent skipped exclusion from source casing before the normalizing boundary exists.
- **Files:** `internal/safety/{safety.go,safety_test.go}`; deleted `playground/slice-05-safety/`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — 8 tests incl. lowercase `cronjob` excluded and uppercase `JVM` cautioned.
- **Codex status:** ✅ approved. The graduated layout matches the prior review, `EqualFold` is good defense in depth for kind/runtime casing, and root-module tests/vet pass.

### #8 — 2026-07-06 — build slice: exclusions + confidence (slice-05 safety)
- **What:** `safety` package — `Assess` (exclude CronJob/Job as bursty-by-design and <7d-history as no-signal, each with a printed reason) and `Score` (confidence high/med/low from history length + CPU burstiness P99>1.5×P95, JVM never high + memory caution). Pure logic, `Meta` decoupled from usage numbers.
- **Why:** the guardrail "cautious not confident, floors in code" — this is the offline-testable half of the safety slice; the read-layer that fills `Meta` from the cluster is blocked here (no live cluster).
- **Files:** `playground/slice-05-safety/{safety.go,safety_test.go}`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — 7 tests: batch/short-history exclusions, normal kept, high/med by burstiness, short-history downgrade, JVM caution.
- **Open for Codex:** the confidence thresholds (1.5× spike, 7d/14d history) are first-pass values marked for tuning; also decide final home (`internal/safety` vs folding into `rightsizing`).
- **Codex status:** ✅ reviewed. `go test ./...` and `go vet ./...` pass from the root module with workspace-local `GOCACHE`. The slice is acceptable playground work; before graduation, keep it as `internal/safety` rather than folding into `rightsizing`, and consider normalizing runtime/kind strings at the read-layer boundary so `jvm`/`JVM` and Kubernetes kind casing do not drift.

### #7 — 2026-07-06 — GRADUATE slice-04 → internal/reporting
- **What:** Codex cleared #6; graduated the dollar table to `internal/reporting` (package `reporting`) and resolved both open decisions: (1) name = `reporting` (will host `--json` output later, not just this table); (2) `Render` now qualifies a workload as `namespace/name` **only on name collision across namespaces**, else bare name — clean default, unambiguous on real clusters. Removed `playground/slice-04-dollar-table/`.
- **Why:** approved work leaves the playground (rule 4); collision-aware labels answer Codex's duplicate-name concern without cluttering the common single-namespace case.
- **Files:** `internal/reporting/{pricing.go,table.go,table_test.go}`; deleted `playground/slice-04-dollar-table/`.
- **Verified:** `go vet ./...` clean; `go test ./...` green — original hand-math/price/render tests plus a new collision test (`team-a/api` vs `team-b/api`, unique `web` stays bare).
- **Codex status:** ✅ approved. The `internal/reporting` name is a good fit for table plus future JSON output, namespace collision rendering addresses the duplicate-name concern, and root-module tests/vet pass.

### #6 — 2026-07-06 — build slice: dollar-ranked table (slice-04)
- **What:** `dollartable` package — `DefaultPrice` (AWS/GCP/Azure list-price defaults, derived from readable per-vCPU-h/per-GB-h rates, unknown→AWS fallback), `Rank` (proposed + monthly waste per workload, stable-sorted by $ desc, headline total), and a plain `Render` table via stdlib `tabwriter`. Imports the graduated `internal/rightsizing`; no cluster needed.
- **Why:** `/loop start` → dollar slice, per user direction. Ranking math is the trust-adjacent part, so it carries the hand-math test; color/CLI wiring deferred (ponytail).
- **Files:** `playground/slice-04-dollar-table/{pricing.go,table.go,table_test.go}` (part of root module — no separate go.mod).
- **Verified:** `go vet ./...` clean; `go test ./...` green — Rank order/total/proposed hand-checked ($426.32 + $103.95 = $530.27), price derivation+fallback, and a Render smoke (header, total, ordering).
- **Codex status:** ✅ reviewed. `go test ./...` and `go vet ./...` pass from the root module with workspace-local `GOCACHE`. The slice is acceptable playground work; before graduation, decide the final package name (`internal/dollartable` vs `internal/reporting`) and whether rendered rows should include namespace for duplicate workload names.

### #5 — 2026-07-06 — GRADUATE slice-03 → internal/rightsizing
- **What:** Codex approved #4, so the recommender graduated out of `playground/` into the real tree. Created root `go.mod` (module `github.com/kubeloop/kubeloop`, go1.23) and `internal/rightsizing/{recommend.go,recommend_test.go}` (package renamed `recommender`→`rightsizing` to match the path the README promises). Removed `playground/slice-03-recommender/`. Added `/.gocache/` to `.gitignore`.
- **Why:** approved work leaves the playground per rule 4. Module path is the brand-canonical `github.com/kubeloop/kubeloop`, not the fork's `kubeloop-` remote — change if a real import path is decided.
- **Files:** `go.mod`, `internal/rightsizing/*`, `.gitignore`; deleted `playground/slice-03-recommender/`.
- **Verified:** `go vet ./...` clean; `go test ./...` 5/5 pass from module root.
- **Codex status:** ✅ approved. The graduated layout preserves the reviewed recommender behavior, the package/path match the README, and root-module tests/vet pass.

### #4 — 2026-07-06 — address Codex #3: realistic CPU test + honest floor semantics
- **What:** replaced the impossible `P95>P99` test. Finding: since real data has P99≥P95, the `P99×1.2` floor *always* governs CPU — the P95 term is a defensive fallback for missing/degenerate P99 (Prometheus gap → P99=0), not a normal path. Documented that in code; tests now cover the real branches: floor-governs (normal), and P95-fallback (P99 missing).
- **Why:** Codex flagged the base-branch test as unrealistic. Chasing it exposed that P95 never wins under realistic ordering — made the code say what actually happens instead of implying P95 is a live knob.
- **Files:** `playground/slice-03-recommender/{recommend.go,recommend_test.go}`.
- **Verified:** `go vet` clean; 5/5 tests pass with `GOCACHE=/Users/karthikbangari/kubeloop/.gocache`.
- **Codex status:** ✅ approved on re-review. The corrected test semantics match realistic percentile ordering and the CPU policy is now documented honestly.

### #3 — 2026-07-06 — build slice: recommender math + dollarization
- **What:** first real build increment — `Percentile` recommender (CPU≈P95 floored at P99×1.2, mem=max+15% floored at max+128Mi) and `MonthlyWaste` dollarization, pure functions, no cluster needed.
- **Why:** `/loop start the build`. Started with the safety-critical trust core because it's fully testable offline against hand math; Go toolchain (go1.23.4) installed to `~/.local/go` since none was present.
- **Files:** `playground/slice-03-recommender/{go.mod,recommend.go,recommend_test.go}`.
- **Verified:** `go vet` clean; 4/4 tests pass, each asserting a hand-computed value (both CPU floor/base branches, both mem branches, waste, and never-negative floor).
- **Codex status:** ✅ reviewed. Tests and vet pass when run with `GOCACHE=/Users/karthikbangari/kubeloop/.gocache`. Before graduation, tighten the CPU base-branch test: true percentile data has P99 ≥ P95, so the current `P95 > P99` case is useful branch coverage but not realistic workload evidence.

### #2 — 2026-07-06 — Codex review + project notes
- **What:** added `CODEX.md`, expanded `playground/README.md`, and reviewed the initial Claude Code setup.
- **Why:** user asked to create my md file, keep playground work recorded in the rulebook, and check the Claude Code update.
- **Files:** `CODEX.md`, `RULEBOOK.md`, `playground/README.md`.
- **Codex status:** ✅ approved. The playground gate is clear and matches the project plan.

### #1 — 2026-07-06 — set up the loop
- **What:** created `RULEBOOK.md` (this file) and `playground/` with its README.
- **Why:** user asked for a rule book + playground + Codex-review gate.
- **Files:** `RULEBOOK.md`, `playground/README.md`.
- **Codex status:** ✅ approved in log entry #2.
