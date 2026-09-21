import Link from "next/link";
import { ArrowRight } from "lucide-react";

import { BlurFade } from "@/components/magicui/blur-fade";
import { CodeBlock } from "@/components/code-block";
import { JsonLd } from "@/components/json-ld";
import { PageHero, Section } from "@/components/section";
import { Badge } from "@/components/ui/badge";
import { absoluteUrl } from "@/lib/site";
import { buildMetadata } from "@/lib/metadata";

export const metadata = buildMetadata({
  title: "Use cases",
  description:
    "Five concrete ways teams use KubeCell: accelerator sandboxes, ephemeral test environments, training labs, edge field labs, and platform landlord mode on shared hardware.",
  path: "/use-cases/",
  keywords: [
    "Kubernetes accelerator sandbox",
    "ephemeral Kubernetes environments",
    "GPU cluster for developers",
    "training lab Kubernetes",
    "platform landlord mode",
  ],
});

type UseCase = {
  id: string;
  title: string;
  summary: string;
  audience: string;
  situation: string;
  kubecell: string;
  outcome: string[];
  code: string;
  filename: string;
};

const useCases: UseCase[] = [
  {
    id: "accelerator-sandboxes",
    title: "Accelerator sandboxes for AI teams",
    summary:
      "Every researcher gets a full Kubernetes API with two dedicated 910B cards — no partitioning tricks, no shared queue.",
    audience: "Research and ML platform teams",
    situation:
      "Your lab runs one or two powerful ARM64 hosts with eight accelerators each. Today, jobs land in a shared namespace: one runaway workload can starve everyone, and nobody can install an operator or a CRD without stepping on another team's cluster.",
    kubecell:
      "Publish a quota tier with two whole cards, then hand out clusters. Each one is a real K3s control plane, so teams install whatever operators they need inside their own API server while the host still enforces the quota.",
    outcome: [
      "Each cluster gets whole cards; two clusters never share one accelerator.",
      "Capacity shown in status is what the hardware actually reports, not what was requested.",
      "Quota usage is measured on the host, so the limits you set are the limits that apply.",
      "Works with any accelerator your cluster advertises, NVIDIA or Ascend alike.",
    ],
    code: `apiVersion: kubecell.io/v1alpha1
kind: VirtualNodeClass
metadata:
  name: ascend-910b-small
spec:
  entitlement:
    workloadHard:
      requests.cpu: "4"
      limits.cpu: "4"
      requests.memory: 8Gi
      limits.memory: 8Gi
      requests.huawei.com/Ascend910: "2"
      limits.huawei.com/Ascend910: "2"
  storageClassName: topolvm-provisioner`,
    filename: "class-ascend-910b-small.yaml",
  },
  {
    id: "ephemeral-environments",
    title: "Ephemeral environments per pull request",
    summary:
      "Spin a child cluster for an integration run, hand over a kubeconfig, then delete it and let the finalizer clean the host.",
    audience: "Teams with heavy integration tests",
    situation:
      "Integration tests need cluster-scoped objects: CRDs, webhooks, storage classes, and a control plane that can be restarted. A shared namespace cannot provide that, and provisioning a whole managed cluster per test is too slow and too expensive.",
    kubecell:
      "A cluster is created in seconds once images are warm, and deletion is complete: KubeCell removes the cluster, its credentials, and everything it created on the host. The kubeconfig is always published to one Secret, so CI can pick it up with a single command.",
    outcome: [
      "One cluster per branch: apply a manifest, wait for Ready, run tests, delete.",
      "An admin kubeconfig is published every time — no manual hand-off.",
      "Predictable cleanup, including PVCs, with Retain as the storage safety net.",
      "Dry-run checks let CI fail fast when the host is out of capacity instead of hanging.",
    ],
    code: `apiVersion: kubecell.io/v1alpha1
kind: VirtualClusterPlan
metadata:
  name: pr-1842-plan
  namespace: kubecell-system
spec:
  cellRef: {name: cell1}
  classRef: {name: ci-medium}
---
# CI waits for status.decision, then creates the real VirtualCluster
apiVersion: kubecell.io/v1alpha1
kind: VirtualCluster
metadata:
  name: pr-1842
  namespace: kubecell-system
spec:
  cellRef: {name: cell1}
  classRef: {name: ci-medium}`,
    filename: "ci-virtualcluster.yaml",
  },
  {
    id: "training-labs",
    title: "Workshops and training labs",
    summary:
      "Forty students, one host, forty clusters. Quota keeps the class fair and no one can break another student's control plane.",
    audience: "Training providers and universities",
    situation:
      "Students need admin rights to learn Kubernetes: install operators, break things, and fix them. Namespace-only classrooms cannot teach cluster-scoped concepts, and per-student VMs are expensive and slow to prepare.",
    kubecell:
      "Create one small quota tier and issue a cluster per student. Each cluster shows a single node, so every kubectl tutorial works verbatim, and the host keeps a misbehaving cluster from affecting the rest of the class.",
    outcome: [
      "Admin kubeconfigs without giving anyone access to the host.",
      "Hard CPU and memory caps per student, enforced by Kubernetes, not by convention.",
      "Students cannot break the host: privileged Pods and hostPath volumes are rejected.",
      "Teardown is one delete per student cluster.",
    ],
    code: `# Create the whole class from a small loop
for student in $(seq 1 40); do
  cat <<EOF | kubectl apply -f -
apiVersion: kubecell.io/v1alpha1
kind: VirtualCluster
metadata:
  name: student-$student
  namespace: kubecell-system
spec:
  cellRef: {name: lab-cell}
  classRef: {name: workshop-small}
EOF
done`,
    filename: "classroom.sh",
  },
  {
    id: "field-labs",
    title: "Edge and field labs",
    summary:
      "Hardware lives where the hardware is. Hosts connect outbound to one management plane — no inbound tunnel per site.",
    audience: "Ops teams running remote sites",
    situation:
      "Your accelerators and data live at a remote site behind NAT. You still need one place to declare changes, watch capacity, and hand out developer clusters — without opening inbound management paths to every machine.",
    kubecell:
      "Hosts connect outbound to the management cluster. Every change is applied through that one channel and every observation comes back the same way, so there is no inbound tunnel to maintain and no agent to babysit.",
    outcome: [
      "One management plane for many sites, with each site isolated on its own hosts.",
      "No inbound SSH or API access to hosts required for day-to-day operation.",
      "Sites that stop reporting are visible immediately, before anyone tries to create a cluster.",
      "Endpoints are discovered and republished, so kubeconfigs keep working after address changes.",
    ],
    code: `kubectl -n kubecell-system get cells -o custom-columns=\\
NAME:.metadata.name,PHASE:.status.phase,JOINED:.status.managedCluster.joined,\\
LEASE:.status.managedCluster.leaseFresh,NODES:.status.nodes[*].name`,
    filename: "fleet-status.sh",
  },
  {
    id: "platform-landlord",
    title: "Landlord mode for platform teams",
    summary:
      "Declare Cells and quota tiers once; product teams self-serve child clusters inside the envelope you defined.",
    audience: "Internal platform teams",
    situation:
      "Requests for clusters arrive faster than your team can provision hardware. You need the platform to own capacity and guardrails while product teams self-serve inside them — without negotiating every request in chat.",
    kubecell:
      "The platform publishes hosts with their observed capacity and a small set of quota tiers. Teams check a request with a dry run, then create the cluster. Every step is visible as a Kubernetes resource with status, so capacity conversations happen against real data.",
    outcome: [
      "Dry runs before commitment: accepted, rejected with reasons, or unknown.",
      "Capacity accounting per host: requested, pending, and available estimates.",
      "Bindings are immutable, so capacity cannot silently move between hosts.",
      "The same tested versions everywhere, upgraded through releases.",
    ],
    code: `# Tenants can inspect headroom before asking for anything
kubectl -n kubecell-system get cell cell1 \\
  -o jsonpath='{range .status.inventory[*]}{.resource}{"\\t"}{.availableEstimate}{"\\n"}{end}'`,
    filename: "capacity.sh",
  },
];

const itemListLd = {
  "@context": "https://schema.org",
  "@type": "ItemList",
  name: "KubeCell use cases",
  itemListElement: useCases.map((useCase, index) => ({
    "@type": "ListItem",
    position: index + 1,
    name: useCase.title,
    description: useCase.summary,
    url: absoluteUrl(`/use-cases/#${useCase.id}`),
  })),
};

export default function UseCasesPage() {
  return (
    <>
      <JsonLd data={itemListLd} />
      <PageHero
        eyebrow="Use cases"
        title="Shared bare metal, private Kubernetes"
        description="KubeCell is for teams whose hardware is too expensive to duplicate and whose workloads are too real for namespaces alone. Here are five patterns we designed for."
      />

      <Section>
        <div className="space-y-20">
          {useCases.map((useCase, index) => (
            <BlurFade key={useCase.id} delay={0.04 * index} inView>
              <article id={useCase.id} className="scroll-mt-24">
                <div className="flex flex-wrap items-center gap-3">
                  <Badge variant="outline" className="border-sky-400/30 text-sky-300">
                    {useCase.audience}
                  </Badge>
                  <span className="font-mono text-xs text-muted-foreground">
                    {String(index + 1).padStart(2, "0")}
                  </span>
                </div>
                <h2 className="mt-4 text-2xl font-semibold tracking-tight text-foreground sm:text-3xl">
                  {useCase.title}
                </h2>
                <p className="mt-3 max-w-3xl text-pretty text-base leading-7 text-muted-foreground">
                  {useCase.summary}
                </p>

                <div className="mt-8 grid gap-8 lg:grid-cols-[1.05fr_0.95fr] lg:gap-12">
                  <div className="space-y-6">
                    <div>
                      <h3 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
                        The situation
                      </h3>
                      <p className="mt-2 text-sm leading-7 text-foreground/85">
                        {useCase.situation}
                      </p>
                    </div>
                    <div>
                      <h3 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
                        With KubeCell
                      </h3>
                      <p className="mt-2 text-sm leading-7 text-foreground/85">{useCase.kubecell}</p>
                    </div>
                    <div>
                      <h3 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
                        What you get
                      </h3>
                      <ul className="mt-2 space-y-2">
                        {useCase.outcome.map((item) => (
                          <li
                            key={item}
                            className="flex gap-2 text-sm leading-6 text-muted-foreground"
                          >
                            <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-sky-400" />
                            {item}
                          </li>
                        ))}
                      </ul>
                    </div>
                  </div>
                  <CodeBlock
                    code={useCase.code}
                    language={useCase.filename.endsWith(".sh") ? "bash" : "yaml"}
                    filename={useCase.filename}
                  />
                </div>
              </article>
            </BlurFade>
          ))}
        </div>

        <div className="mt-20 rounded-2xl border border-white/10 bg-white/[0.02] p-8 text-center">
          <h2 className="text-xl font-semibold text-foreground">
            Do not see your scenario?
          </h2>
          <p className="mx-auto mt-3 max-w-2xl text-sm leading-6 text-muted-foreground">
            If it needs a real Kubernetes API on hardware you already own, it is probably a fit.
            Read the technical design or open an issue with your constraints.
          </p>
          <div className="mt-6 flex flex-wrap items-center justify-center gap-4 text-sm font-medium">
            <Link href="/architecture/" className="text-sky-400 hover:text-sky-300">
              Technical principles
            </Link>
            <span className="text-white/10">|</span>
            <Link href="/quickstart/" className="text-sky-400 hover:text-sky-300">
              Quickstart
            </Link>
            <span className="text-white/10">|</span>
            <a
              href="https://github.com/taosher/KubeCell/issues"
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-1 text-sky-400 hover:text-sky-300"
            >
              Open an issue
              <ArrowRight className="h-3.5 w-3.5" />
            </a>
          </div>
        </div>
      </Section>
    </>
  );
}
