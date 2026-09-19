# Kubebuilder workflow essentials (scaffolding + regeneration)

Use this reference when the user asks about Kubebuilder project setup and regeneration steps (CRDs, RBAC, manifests), but does *not* want you to implement controller/webhook business logic.

Canonical background on what Kubebuilder generates (and why):
[`book.kubebuilder.io/reference/generating-crd.html`](https://book.kubebuilder.io/reference/generating-crd.html).

## What to clarify first

- Kubebuilder version (or Operator SDK version if they use that wrapper)
- Are they creating a new project or modifying an existing one?
- Single-group vs multi-group layout
- Namespaced vs cluster-scoped resource(s)

## Minimal end-to-end workflow (most common)

1. Initialize the repo (new project only)
   - Run: `kubebuilder init --domain <domain> --repo <go-module>`
   - Ensure `go.mod` module path matches `--repo`.

2. Scaffold an API (CRD + controller skeleton)
   - Run: `kubebuilder create api --group <group> --version <version> --kind <Kind>`
   - If they only want CRD types and not a reconciler yet, they can still scaffold and ignore controller logic, or use flags if their Kubebuilder version supports skipping controller/webhook generation.

3. Edit API types
   - Edit `api/<version>/<kind>_types.go` (Spec/Status, markers).
   - Keep API types stable and compatible; add new fields as optional unless they are truly required.

4. Regenerate generated code and manifests
   - Run: `make generate`
   - Run: `make manifests`

5. Verify the generated CRD YAML
   - Inspect `config/crd/bases/<group>_<plural>.yaml`.
   - Confirm:
     - required fields and defaults
     - validation schema matches expectations (patterns, enums, min/max)
     - list/map semantics (`x-kubernetes-list-type`, `x-kubernetes-list-map-keys`)
     - status subresource exists when needed

## CRD generation concepts that commonly trip teams up

- **Subresources are opt-in**:
  - `/status` should be enabled for almost any resource with a `.status` field (root marker: `+kubebuilder:subresource:status`).
  - `/scale` is opt-in and requires mapping paths (marker: `+kubebuilder:subresource:scale:specpath=...,statuspath=...,selectorpath=...`).

- **Printer columns come from markers**:
  - `+kubebuilder:printcolumn` on the root type emits `additionalPrinterColumns`.
  - Prefer stable JSONPaths (avoid deep arrays unless you really mean it).

- **Multi-version CRDs are a workflow, not just a directory layout**:
  - If you add versions, you typically need conversion (webhook) and a storage version marker.
  - See the pointer section in [`SKILL.md`](../SKILL.md).

- **controller-gen flags matter**:
  - Kubebuilder’s `Makefile` drives `controller-gen` with options (often via `CRD_OPTIONS`).
  - If generated schemas don’t match expectations across versions, inspect the `Makefile` and the pinned controller-gen version.

## Common pitfalls to call out

- “Required” vs “optional” in Go:
  - Optional scalars should usually be pointers.
  - `omitempty` affects JSON output; requiredness is driven by CRD schema, not omitempty alone.
- Defaults:
  - Defaults must be defined in the schema (markers), and they only apply on create.
- Lists and maps:
  - For server-side apply correctness, define list semantics (`listType=set|map`, `listMapKey=...`).
- References/relations:
  - Prefer API-owned reference structs over embedding generic Kubernetes reference types.

## When the user hits generation issues

- Re-run `make generate` and `make manifests` after any API type change.
- If schema markers do not show up, confirm they are on the correct field/type and supported by the controller-gen version in the project.
- If CRDs won’t apply, look for structural schema violations (e.g., missing `type` in schema, invalid defaults).
