# Kubebuilder markers cheat-sheet (for Go CRD types)

Use this file when you need to translate API requirements into `+kubebuilder` markers on Go structs.

For canonical background on marker syntax and how Kubebuilder runs generation, see:

- [`book.kubebuilder.io/reference/markers.html`](https://book.kubebuilder.io/reference/markers.html)
- [`book.kubebuilder.io/reference/generating-crd.html`](https://book.kubebuilder.io/reference/generating-crd.html)

## Contents

- Root object markers
- Field-level validation and defaults
- List/map semantics (SSA-safe)
- Printer columns

## Marker syntax (quick canonical notes)

Controller-gen markers are single-line comments that start with `// +`.

Common forms:

- **Empty** flag-like markers:

```go
// +kubebuilder:validation:Optional
```

- **Anonymous** single-value markers:

```go
// +kubebuilder:validation:MaxItems=2
```

- **Multi-option** markers (comma-separated named args; order doesn’t matter):

```go
// +kubebuilder:printcolumn:JSONPath=".status.replicas",name=Replicas,type=string
```

Value syntax is Go-like (bools, ints, strings). Prefer quoting strings unless they are single-word.

Difference to remember:

- `// +optional` is a Kubernetes codegen convention and is often inferred from `omitempty`.
- `// +kubebuilder:validation:Optional` is a controller-gen marker and can also be used package-wide.

## Root object markers

Add these above the root type:

```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=foo,categories=all
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
type Foo struct {
  // ...
}
```

Common:

- `+kubebuilder:subresource:status` if you have `.status`.
- `+kubebuilder:resource:scope=Cluster` for cluster-scoped.

## Field-level markers

### Required vs optional

Prefer OpenAPI-required (not `omitempty`) for required fields.

- Required field: no `omitempty`, non-pointer (or pointer with `+kubebuilder:validation:Required` only if you must)
- Optional field: `omitempty` and usually pointer for scalars

### Strings

```go
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=63
// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
// +kubebuilder:validation:Enum=small;medium;large
Name string `json:"name"`
```

### Numbers

```go
// +kubebuilder:validation:Minimum=0
// +kubebuilder:validation:Maximum=10
// +kubebuilder:validation:MultipleOf=5
Replicas *int32 `json:"replicas,omitempty"`
```

### Defaults

Defaults must be valid JSON literals.

```go
// +kubebuilder:default:=3
Replicas *int32 `json:"replicas,omitempty"`

// +kubebuilder:default:="Always"
Policy string `json:"policy,omitempty"`
```

### Objects / nested structs

```go
// +kubebuilder:validation:XValidation:rule="has(self.foo)",message="foo is required"
Config *FooConfig `json:"config,omitempty"`
```

Note: advanced rules vary by Kubernetes version; use sparingly.

## Lists and maps (important)

### List item validation

```go
// +kubebuilder:validation:MinItems=1
// +kubebuilder:validation:MaxItems=10
Items []Item `json:"items,omitempty"`
```

### List semantics for SSA

Use these markers to declare the list semantics you want in the generated CRD schema. For the contract impact and GitOps/SSA review heuristics, see the canonical reference in [`list-semantics-gitops-ssa.md`](../../k8s-crd-design-review/references/list-semantics-gitops-ssa.md):

- Sets (unique items):

```go
// +kubebuilder:validation:UniqueItems=true
// +kubebuilder:validation:MaxItems=50
// +kubebuilder:validation:items:Pattern=`^[a-z0-9-]+$`
// +listType=set
Tags []string `json:"tags,omitempty"`
```

- Map-like lists (keyed by a field):

```go
// +listType=map
// +listMapKey=name
Endpoints []Endpoint `json:"endpoints,omitempty"`
```

Special case: `metav1.Condition` lists should almost always be map-like, keyed by `type`:

```go
// +listType=map
// +listMapKey=type
Conditions []metav1.Condition `json:"conditions,omitempty"`
```

For more depth on list semantics, see [`list-semantics-gitops-ssa.md`](../../k8s-crd-design-review/references/list-semantics-gitops-ssa.md:1).

## Printer columns

Prefer stable JSONPaths. Keep them short and user-facing.

Examples:

```go
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`,priority=0
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.message`,priority=1
```
