# playground

Scratch/experiment area. Build here first.

Nothing here is real until Codex reviews it and it graduates per [`../RULEBOOK.md`](../RULEBOOK.md).

## How to use this directory

- Keep experiments small and named by slice, for example `slice-03-read-layer/`.
- Add a short README inside each experiment that says what it tests, how to run it, and what result would make it graduate.
- Do not put production code directly in the root project until the rulebook log says it has passed review.
- Delete or archive failed experiments only after noting the decision in `../RULEBOOK.md`.

## Current status

Active experiment:

- None right now. Start the next slice here and add a matching entry to `../RULEBOOK.md`.

Graduated:

- `slice-03-recommender/` moved to `../internal/rightsizing/`.
- `slice-04-dollar-table/` moved to `../internal/reporting/`.
- `slice-05-safety/` moved to `../internal/safety/`.
- `slice-06-scan/` moved to `../internal/scan/`.
- `slice-07-cli/` moved to `../cmd/kubeloop/`.
- `slice-08-savings/` moved to `../internal/savings/`.
- `slice-09-pricing/` moved into `../internal/reporting/` and `../cmd/kubeloop/`.
- `slice-10-labels/` moved to `../internal/labels/`.
- `slice-11-inventory/` moved to `../internal/inventory/`.
- `slice-12-runtime/` folded into `../internal/inventory/`.
- `slice-13-offline-assembly/` moved to `../internal/readlayer/`.
- `slice-14-promusage/` moved to `../internal/readlayer/promusage/`.
- `slice-15-patcher/` and `slice-16-prcompose/` moved to `../internal/pr/`.
- `slice-17-locator/` and `slice-18-prprepare/` moved to `../internal/pr/`.
- `slice-19-quantity/` moved to `../internal/pr/quantity/`.
- `slice-20-reduceonly/` and `slice-21-rowselect/` moved to `../internal/pr/`.
- `slice-24-promclient/` moved to `../internal/readlayer/promclient/`.
- `slice-25-quantityparse/` moved to `../internal/readlayer/quantityparse/`.
- `slice-26-kubeparse/` moved to `../internal/readlayer/kubeparse/`.
- `slice-27-manifestsource/` moved to `../internal/readlayer/manifestsource/`.
- `slice-28-dirsource/` moved to `../internal/readlayer/dirsource/`; now backs the CLI's `--from-manifests` mode.
- `slice-29-kubeclient/` moved to `../internal/readlayer/kubeclient/`.
- `slice-30-promql/` moved to `../internal/readlayer/promql/`.
- `slice-31-clustersource/` moved to `../internal/readlayer/clustersource/`; the three back the CLI's `--from-cluster` mode.
- `slice-32-gitrepo/` moved to `../internal/pr/gitrepo/`.
- `slice-33-ghclient/` moved to `../internal/pr/ghclient/`.
- `slice-34-openpr/` moved to `../internal/pr/openpr/`; the three back the CLI's `pr --open` mode.
