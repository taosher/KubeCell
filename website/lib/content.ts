export const pinnedStack = [
  {
    component: "Management cluster",
    version: "Single-node K3s",
    note: "Holds your manifests and reports state; runs no tenant workloads.",
  },
  {
    component: "Host Kubernetes",
    version: "K3s v1.36.3+k3s1",
    note: "Validated on ARM64 bare metal.",
  },
  {
    component: "K3k",
    version: "v1.2.0",
    note: "Creates and manages the per-developer control planes on each host.",
  },
  {
    component: "Cluster Kubernetes",
    version: "v1.34.2+k3s1",
    note: "The version your developers see and target with kubectl.",
  },
  {
    component: "OCM Hub",
    version: "v1.3.1",
    note: "The audited channel between the management cluster and hosts.",
  },
  {
    component: "Storage",
    version: "TopoLVM on hosts",
    note: "Persistent volumes for clusters, backed by local disks on the host.",
  },
] as const;

export const isolationControls = [
  {
    title: "A dedicated namespace per cluster",
    description:
      "Your workloads run in their own namespace on the host, with quotas, defaults, and network rules attached to it.",
  },
  {
    title: "Hard CPU, memory, and GPU limits",
    description:
      "Quotas come from the tier you pick and are enforced by Kubernetes itself, not by convention or a dashboard.",
  },
  {
    title: "Private by default",
    description:
      "Clusters cannot reach each other's services unless you explicitly allow it.",
  },
  {
    title: "Unsafe workloads are blocked",
    description:
      "Privileged containers and hostPath volumes are rejected before they reach a host, with a hint about what to use instead.",
  },
  {
    title: "Nothing changes behind your back",
    description:
      "Every host-side change is a declarative manifest applied through one management plane, so you can see and review what happened.",
  },
  {
    title: "Whole GPUs, never shared",
    description:
      "Accelerators are allocated as whole cards: your cluster gets the card, or it does not. No oversubscription between tenants.",
  },
] as const;
