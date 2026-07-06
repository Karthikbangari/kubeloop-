# CODEX — kubeloop working notes

This is Codex's project note file for this repo.

## Current understanding

kubeloop is planned as a read-only Kubernetes rightsizing CLI that turns resource waste into dollar-ranked recommendations, then graduates approved changes through Git pull requests instead of mutating the cluster.

The repo is currently in the foundation stage:

- Product/spec docs are in `README.md`, `docs/`, and `plan/MASTER-PLAN-LOOPED.md`.
- Local validation assets live under `dev/`.
- The test cluster is driven by `Makefile`.
- `deploy/rbac.yaml` defines read-only Kubernetes access.
- `playground/` is the scratch space where implementation experiments start before graduating.

## Working rule

Follow `RULEBOOK.md`. Build experiments in `playground/` first, log the change in `RULEBOOK.md`, review it, then graduate only the approved pieces into the real project tree.

Whenever Claude Code changes the repo, check `playground/` automatically before doing anything else: read the latest rulebook entry, inspect the changed files, run relevant checks, and update the review status in `RULEBOOK.md`.
