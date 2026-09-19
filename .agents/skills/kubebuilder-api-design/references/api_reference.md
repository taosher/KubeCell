# Kubebuilder markers cheat-sheet (for Go CRD types)

Use this file when you need to translate API requirements into `+kubebuilder` markers on Go structs.

## Contents

- Root object markers
- Field-level validation and defaults
- List/map semantics (SSA-safe)
- Printer columns

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

### Required vs optional (where markers are *not* the main tool)

Whether a field is required is primarily expressed via Go types + `json` tags (pointer vs non-pointer, `omitempty` vs no `omitempty`).

Keep the rules in one place: see [`go-type-patterns.md`](./go-type-patterns.md:1).

Use this file mainly for validation/defaulting markers once you have the Go shape.

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

Use these on list fields to declare the intended set/map semantics in the generated CRD schema (for the contract impact and GitOps/SSA review, see the canonical reference in [`list-semantics-gitops-ssa.md`](../../k8s-crd-design-review/references/list-semantics-gitops-ssa.md:1)):

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

For more depth on list semantics, see [`list-semantics-gitops-ssa.md`](../../k8s-crd-design-review/references/list-semantics-gitops-ssa.md:1).

## Printer columns

Prefer stable JSONPaths. Keep them short and user-facing.

Examples:

```go
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`,priority=0
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.message`,priority=1
```
