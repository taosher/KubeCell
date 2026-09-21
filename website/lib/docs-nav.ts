export type DocLink = {
  title: string;
  href: string;
  description?: string;
};

export type DocNavGroup = {
  label: string;
  items: DocLink[];
};

export const docsNav: DocNavGroup[] = [
  {
    label: "Introduction",
    items: [
      {
        title: "Overview",
        href: "/docs/",
        description: "What KubeCell is, who it is for, and how the pieces fit together.",
      },
      {
        title: "Core concepts",
        href: "/docs/concepts/",
        description: "The four KubeCell resources, quota tiers, and how a cluster maps to a host.",
      },
    ],
  },
  {
    label: "Getting started",
    items: [
      {
        title: "Installation",
        href: "/docs/installation/",
        description: "Install the management cluster, connect a host, and prepare it for tenants.",
      },
      {
        title: "First VirtualCluster",
        href: "/docs/first-virtualcluster/",
        description: "Preflight a request, create your first cluster, and get the kubeconfig.",
      },
      {
        title: "Host onboarding",
        href: "/docs/host-onboarding/",
        description: "The four-command flow that adds a physical host to KubeCell.",
      },
    ],
  },
  {
    label: "Guides",
    items: [
      {
        title: "Day-2 operations",
        href: "/docs/operations/",
        description: "Inventory checks, credentials, quota changes, ingress, upgrades, deletion.",
      },
      {
        title: "API reference",
        href: "/docs/api-reference/",
        description: "Every CRD field, immutability rule, and status condition.",
      },
      {
        title: "Security & isolation",
        href: "/docs/security/",
        description: "Namespace, quota, policy, and admission controls per child cluster.",
      },
      {
        title: "Troubleshooting",
        href: "/docs/troubleshooting/",
        description: "Symptom-first diagnostics across Cell, Work, child API, and storage.",
      },
      {
        title: "FAQ",
        href: "/docs/faq/",
        description: "Short answers about scope, limits, and design trade-offs.",
      },
    ],
  },
];

export const docsFlat: DocLink[] = docsNav.flatMap((group) => group.items);

export function docNeighbors(pathname: string) {
  const index = docsFlat.findIndex((item) => item.href === pathname);
  return {
    previous: index > 0 ? docsFlat[index - 1] : undefined,
    next: index >= 0 && index < docsFlat.length - 1 ? docsFlat[index + 1] : undefined,
  };
}
