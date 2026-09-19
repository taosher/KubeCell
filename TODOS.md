# Roadmap

> Design authority is `technical-design.md` at the repository root. Update the design first, then implement.
> Priorities: P0 blocks the main line or release safety; P1 improves onboarding and usability; P2 strengthens defaults, validation, and known-issue follow-ups.

## Milestones

| Milestone | Goal | Exit criteria |
| --- | --- | --- |
| M1 API shape | Cell / VirtualNodeClass / VirtualClusterPlan / VirtualCluster types, validation, immutability, and namespace rules | `controller-gen`, `go build ./api/...`, and `go test ./api/...` pass; CRD diff reviewed |
| M2 Controllers | Reconciliation, rendering, feasibility evaluation, and host admission | Unit tests and envtest pass; Plan Accepted/Rejected/Unknown reproducible locally; `make manifests generate` clean |
| M3 Installer and release bundle | Repeatable install with pinned versions and read-only preflight | Bundle builds, digest verification passes, `doctor --dry-run` is human-reviewable |
| M4 First end-to-end loop | One Ready VirtualCluster from a fresh bundle | VirtualCluster Ready; admin kubeconfig works; CPU Pod reflects and quota accounting holds |
| M5 Feature matrix | PVC, networking, hostPath rejection, interaction, cascading deletion, accelerator allocation, Ingress | CI feature matrix passes |
| M6 Dashboard | Management-plane visibility | Dashboard login works and Cell/VirtualCluster resources are visible |
| M7 Release hardening | Repeatable release from zero without manual rescue | Full reinstall loop passes; docs have no open placeholders or only explicitly marked ones |

## Task List

| ID | Item | Status | Priority |
| --- | --- | --- | --- |
| P0-001 | Freeze `technical-design.md` as the single source of truth | Done | P0 |
| P0-002 | Pin upstream versions and enforce them in code and bundles | Done | P0 |
| P1-001 | Generate Cell from live host discovery | Done | P1 |
| P1-002 | Ship host baseline command templates in the release bundle | Done, vendor real artifacts in M7 | P1 |
| P1-003 | Suggest VirtualNodeClass drafts from observed allocatable capacity | Done | P1 |
| P1-004 | Document host onboarding in four commands | Done, see `docs/operations/host-onboarding.md` | P1 |
| P1-005 | Bundle the dashboard as an optional management-chart dependency | Done | P1 |
| P2-001 | Version-drift alerting for the locked combination | Done via platform constants and observed conditions | P2 |
| P2-002 | Harden webhook defaults and validation coverage | In progress, tracked by issue | P2 |
| P2-003 | Digest known-issue follow-ups behind design updates | In progress, tracked by issue | P2 |

## Iteration Discipline

| Action | When |
| --- | --- |
| Local gates (`make manifests generate`, `go test ./...`, envtest, chart checks) | Every commit |
| Light loop (reinstall charts, recreate Cell/Class/VirtualCluster) | Controller, rendering, or admission changes |
| Full loop (`docs/testing/ci-plan.md`: uninstall + register + install + feature matrix + evidence) | Installer, OCM, or host-baseline changes, or at least weekly |
| Evidence under `.out/<run>/` (gitignored) | Every loop |
