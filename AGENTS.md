# KubeCell Repository Guidelines

## Project Basics

- KubeCell is implemented in Go and delivered as a Kubernetes Operator with supporting components.
- The project uses Kubebuilder as the scaffolding and code-generation framework.
- New APIs, controllers, admission webhooks, RBAC markers, generated manifests, and generated code must follow Kubebuilder conventions.
- Any change that can be produced from API definitions, markers, or generation commands must never be hand-edited in generated files.
- Controller reconciliation must be idempotent and safe to retry.

## Language

- All Git commit messages are written in English using Conventional Commits (`feat`/`fix`/`docs`/`refactor`/`test`/`chore`, imperative mood, subject line up to 72 chars).
- All technical documentation is written in English.
- Scope includes architecture, design, operations, guides, API references, release notes, and technical comments in samples.
- All code comments are written in English.

## Documentation and Design

- `technical-design.md` at the repository root is the single source of truth for all subsequent implementation. Any change to behavior, APIs, dependencies, or operational assumptions must first update `technical-design.md` and get it reviewed before changing code. The same applies when implementation reveals a design gap. Do not deviate from the design for implementation convenience.
- When behavior, APIs, dependencies, or operational assumptions change, update the related documentation in the same change.
- Clearly distinguish KubeCell-owned APIs, Kubernetes native resources, and third-party CRDs.

## Change Discipline

- Keep a no-fork policy for upstream projects such as K3k, K3s, TopoLVM, Multus, and Whereabouts. Integrate through public APIs, standard Kubernetes resources, adapters, or sidecar/Operator mechanisms.
- Prefer existing Kubernetes scheduling, Device Plugin, CNI, CSI, RBAC, and lifecycle mechanisms. Do not build a second scheduler or a duplicate resource allocation system.
- Any change to reconciliation, defaults, validation, recovery, or deletion/Finalizer behavior must add matching tests.
- Follow the CI process in `docs/testing/ci-plan.md` before merging major features.

## Experiment Discipline

- Smoke scripts and one-off experiment output always go under `.out/` at the repository root (gitignored, never committed), never back into `docs/`.
- Only knowledge accepted as long-lived operations guidance is organized into English documents under `docs/`.
- Remove stale experiments instead of accumulating them under `docs/`.
- Track bugs through GitHub issues with reproduction steps, expected vs. actual behavior, and related resources.
