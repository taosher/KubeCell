export const siteConfig = {
  name: "KubeCell",
  tagline: "A Kubernetes cluster for every developer, on hardware you already own",
  title: "KubeCell — A Kubernetes cluster for every developer, on shared bare metal",
  description:
    "KubeCell gives every developer their own isolated Kubernetes cluster on the servers you already run. Standard kubectl, hard CPU/memory/GPU quotas, persistent storage, and a kubeconfig in minutes.",
  shortDescription:
    "Give every developer their own isolated Kubernetes cluster on shared hardware — real kubectl, hard quotas, ready in minutes.",
  url: process.env.NEXT_PUBLIC_SITE_URL ?? "https://kubecell.pages.dev",
  github: "https://github.com/taosher/KubeCell",
  githubIssues: "https://github.com/taosher/KubeCell/issues",
  docs: {
    design: "https://github.com/taosher/KubeCell/blob/main/technical-design.md",
    onboarding:
      "https://github.com/taosher/KubeCell/blob/main/docs/operations/host-onboarding.md",
    ciPlan: "https://github.com/taosher/KubeCell/blob/main/docs/testing/ci-plan.md",
  },
  keywords: [
    "KubeCell",
    "Kubernetes operator",
    "virtual cluster",
    "K3s",
    "K3k",
    "multi-tenancy",
    "bare metal Kubernetes",
    "resource quota isolation",
    "virtual node",
    "TopoLVM",
    "OCM",
    "accelerator sharing",
    "Ascend 910B",
    "GPU sharing Kubernetes",
    "platform engineering",
    "developer sandbox",
  ],
} as const;

export function absoluteUrl(path = "/") {
  return new URL(path, siteConfig.url).toString();
}

export const primaryNav = [
  { title: "Quickstart", href: "/quickstart/" },
  { title: "Use cases", href: "/use-cases/" },
  { title: "Architecture", href: "/architecture/" },
  { title: "Docs", href: "/docs/" },
] as const;
