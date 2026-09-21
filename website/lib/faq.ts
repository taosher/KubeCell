export type FaqItem = {
  question: string;
  answer: string;
};

export const faqItems: FaqItem[] = [
  {
    question: "What exactly do I get when I create a cluster?",
    answer:
      "A real Kubernetes cluster with its own API server, control plane, and persistent data volume — not a namespace with extra labels. You receive an admin kubeconfig, and normal kubectl, Helm, and CI tools work exactly as they do on any other cluster.",
  },
  {
    question: "Do I need GPUs or accelerators to use KubeCell?",
    answer:
      "No. Accelerators are optional. A host that only offers CPU and memory works fine; you simply publish a quota tier without device requests. The same quota, storage, and isolation model applies either way.",
  },
  {
    question: "Can one developer's workloads starve another's?",
    answer:
      "No. Each cluster has a hard quota for CPU, memory, and accelerators, enforced by Kubernetes on the host. Requests and limits are both capped, so one busy cluster cannot consume another's share.",
  },
  {
    question: "Why does my cluster show only one node?",
    answer:
      "The node is how your cluster sees its quota; the physical machines belong to the host and are managed for you. Need more capacity? Move the cluster to a larger quota tier rather than adding nodes.",
  },
  {
    question: "What happens when I delete a cluster?",
    answer:
      "KubeCell removes the cluster, its credentials, and everything it created on the host. Persistent volumes follow the storage reclaim policy you configured: under Retain they are kept for recovery, so data is never deleted silently.",
  },
  {
    question: "Can two clusters share one GPU?",
    answer:
      "No. Accelerators are allocated as whole cards: a cluster either owns a card or it does not. That keeps performance predictable and makes capacity accounting honest for everyone.",
  },
  {
    question: "Do I need to change my manifests or tooling?",
    answer:
      "No. Workloads inside a cluster are ordinary Pods, Services, Ingresses, and PVCs. There is no sidecar to install and no custom client: your existing kubectl, Helm charts, and CI pipelines work unchanged.",
  },
  {
    question: "How do I expose an app to the internet?",
    answer:
      "Create a normal Ingress in your cluster and choose a name. KubeCell publishes it on the host's shared ingress at a predictable hostname, and TLS is handled at the host level. Ask your platform team for the domain suffix they configured.",
  },
  {
    question: "How long does it take for a cluster to be ready?",
    answer:
      "The first cluster on a host usually takes 10 to 20 minutes because the container images have to be pulled onto the machine. Later clusters start much faster since the layers are already cached.",
  },
  {
    question: "Is KubeCell production ready?",
    answer:
      "Not yet. KubeCell is under active development and the APIs are still v1alpha1. Use it for development, evaluation, and lab environments while the remaining hardening work completes.",
  },
];
