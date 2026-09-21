import Link from "next/link";
import { ArrowRight, Boxes, CloudCog, Layers, Server } from "lucide-react";

import { ArchitectureBeam } from "@/components/home/architecture-beam";
import { CodeBlock } from "@/components/code-block";
import { JsonLd } from "@/components/json-ld";
import { FailureModes } from "@/components/architecture/failure-modes";
import { OrbitingStack } from "@/components/architecture/orbiting-stack";
import { RepoTree } from "@/components/architecture/repo-tree";
import { PageHero, Section, SectionHeading } from "@/components/section";
import { Badge } from "@/components/ui/badge";
import { pinnedStack } from "@/lib/content";
import { absoluteUrl } from "@/lib/site";
import { buildMetadata } from "@/lib/metadata";

export const metadata = buildMetadata({
  title: "Architecture and technical principles",
  description:
    "How KubeCell works under the hood: the three layers, how a request becomes changes on a host, how state stays trustworthy, and which component versions each release validates.",
  path: "/architecture/",
  type: "article",
  keywords: [
    "KubeCell architecture",
    "OCM ManifestWork",
    "K3k shared mode",
    "Kubernetes operator design",
    "TopoLVM reflection",
    "pinned Kubernetes versions",
  ],
});

const layers = [
  {
    icon: CloudCog,
    name: "Management cluster",
    summary: "Single-node K3s. Desired state only; it runs no tenant workloads.",
    points: [
      "OCM Hub: ManagedCluster, ManifestWork, ManagedServiceAccount, cluster proxy.",
      "KubeCell management controller plus the CRD admission webhook.",
      "Four CRDs: Cell, VirtualNodeClass, VirtualClusterPlan, VirtualCluster.",
      "Installer orchestrates release bundles; charts never mutate OCM objects.",
    ],
  },
  {
    icon: Server,
    name: "Cell host cluster",
    summary: "One Cell equals one OCM ManagedCluster equals one physical host cluster.",
    points: [
      "OCM klusterlet and work-agent apply the two works per child cluster.",
      "K3k v1.2.0 in shared mode creates and reflects child clusters.",
      "Host webhook admits platform Pods and rejects tenant host access.",
      "Pinned baseline: CNI/CoreDNS, TopoLVM, Traefik, vendor Device Plugins.",
    ],
  },
  {
    icon: Boxes,
    name: "VirtualCluster (child)",
    summary: "A real K3s control plane with one logical node named kubelet.",
    points: [
      "Independent API server and data volume, stored on TopoLVM.",
      "Child admin kubeconfig always published to a management-plane Secret.",
      "Child Pods and PVCs are reflected to the host namespace one-to-one.",
      "Scale vertically by switching quota tiers, never horizontally.",
    ],
  },
];

const controlFlow = [
  {
    title: "Resolve and snapshot",
    body: "The controller resolves the Cell and VirtualNodeClass, then writes an immutable resolution snapshot to status.resolved. All steady-state rendering reads only from that snapshot.",
  },
  {
    title: "Re-run feasibility",
    body: "Before creating anything, the controller re-runs the same feasibility evaluation the Plan used. A non-Accepted conclusion stops the flow and the report lands in status.feasibilityChecks.",
  },
  {
    title: "Create the Foundation Work",
    body: "Host namespace with policy labels, ResourceQuota, LimitRange, NetworkPolicy, and port-forward RBAC. Only when the work is Applied does the next step start.",
  },
  {
    title: "Create the Instance Work",
    body: "A K3k Cluster in Shared mode plus the K3k VirtualClusterPolicy. The work-agent on the host executes it; K3k then creates the server Pod, data volume, and NodePort Service.",
  },
  {
    title: "Publish credentials",
    body: "The controller reads the K3k kubeconfig Secret through the OCM proxy, rewrites the server address to the auto-discovered host address and NodePort, then republishes it in the management plane.",
  },
  {
    title: "Converge and report",
    body: "Steady-state reconciliation compares observed host state with the snapshot. Drift, stale inventory, and version mismatches surface as conditions — never as silent fixes.",
  },
];

const foundationWork = `# Foundation Work (excerpt) — host namespace and guardrails
apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: kubecell-<vc>-foundation
  namespace: <managed-cluster-namespace>
spec:
  workload:
    manifests:
    - apiVersion: v1
      kind: Namespace
      metadata:
        name: <vc>-ns
        labels:
          kubecell.io/virtualcluster: <vc>
          kubecell.io/managed: "true"
    - apiVersion: v1
      kind: ResourceQuota      # hard requests/limits from the class
    - apiVersion: v1
      kind: LimitRange         # default 100m / 256Mi
    - apiVersion: networking.k8s.io/v1
      kind: NetworkPolicy      # deny by default
# Instance Work (excerpt) — K3k shared-mode cluster
    - apiVersion: k3k.io/v1beta1
      kind: Cluster
      spec:
        mode: shared
        servers: 1
        agents: 1
        persistence:
          type: dynamic
          storageClassName: topolvm-provisioner`;

const freshnessChecks = [
  {
    check: "OCM Joined",
    meaning: "The klusterlet on the host registered successfully.",
    staleRisk: "Registration can be revoked or replaced.",
  },
  {
    check: "OCM Available",
    meaning: "The hub considers the cluster reachable.",
    staleRisk: "Observed to go stale without updates; not trusted alone.",
  },
  {
    check: "managed-cluster-lease fresh",
    meaning: "The host Lease keeps renewing.",
    staleRisk: "The decisive liveness signal for a Cell.",
  },
  {
    check: "InventoryFresh",
    meaning: "Inventory and storage observations are recent and from a live source.",
    staleRisk: "When false, no feasibility conclusion is trustworthy.",
  },
];

const nonGoals = [
  "No custom scheduler, CNI, CSI, device allocator, or host agent: standard Kubernetes behavior applies.",
  "No automatic cross-Cell placement, migration, or failover in the first version.",
  "No strong-adversary multi-tenancy: developer credentials are child-cluster admin kubeconfigs.",
  "port-forward and automatic physical data reclamation are outside the locked commitments.",
];

const articleLd = {
  "@context": "https://schema.org",
  "@type": "TechArticle",
  headline: "KubeCell architecture and technical principles",
  description:
    "Three-layer architecture, OCM ManifestWork control flow, snapshot reconciliation, endpoint discovery, and pinned upstream versions.",
  url: absoluteUrl("/architecture/"),
  author: { "@type": "Organization", name: "KubeCell" },
  about: ["Kubernetes", "K3k", "Open Cluster Management", "TopoLVM"],
  proficiencyLevel: "Expert",
};

export default function ArchitecturePage() {
  return (
    <>
      <JsonLd data={articleLd} />
      <PageHero
        eyebrow="Technical principles"
        title="Small control plane, honest data plane"
        description="KubeCell is deliberately boring: a controller that writes two OCM ManifestWorks per child cluster, reads hosts only through the OCM proxy, and treats upstream projects as the real implementation."
      >
        <Badge variant="outline" className="border-sky-400/30 text-sky-300">
          For platform engineers and reviewers
        </Badge>
        <Link
          href="https://github.com/taosher/KubeCell/blob/main/technical-design.md"
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1.5 text-sm font-medium text-sky-400 transition-colors hover:text-sky-300"
        >
          Read the full design document
          <ArrowRight className="h-3.5 w-3.5" />
        </Link>
      </PageHero>

      <Section>
        <SectionHeading
          eyebrow="Three layers"
          title="One management cluster, many Cells, many child clusters"
          description="Each layer has a single job, and boundaries are enforced by the API surface rather than by convention."
        />
        <div className="mt-12 grid gap-4 lg:grid-cols-3">
          {layers.map((layer) => (
            <div
              key={layer.name}
              className="rounded-2xl border border-white/10 bg-white/[0.02] p-6"
            >
              <span className="inline-flex h-11 w-11 items-center justify-center rounded-xl border border-sky-400/20 bg-sky-500/10 text-sky-300">
                <layer.icon className="h-5 w-5" />
              </span>
              <h3 className="mt-4 text-lg font-semibold text-foreground">{layer.name}</h3>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">{layer.summary}</p>
              <ul className="mt-4 space-y-2">
                {layer.points.map((point) => (
                  <li key={point} className="flex gap-2 text-sm leading-6 text-foreground/80">
                    <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-sky-400/80" />
                    {point}
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>

        <div className="mt-12">
          <ArchitectureBeam variant="technical" />
        </div>
      </Section>

      <Section className="border-y border-white/5 bg-white/[0.015]">
        <SectionHeading
          eyebrow="Control flow"
          title="What happens between kubectl apply and a kubeconfig"
          description="Every step is idempotent and safe to retry. Failed work is retried by re-rendering from the snapshot, not by patching host state in place."
        />
        <div className="mt-12 grid gap-8 lg:grid-cols-[1.1fr_0.9fr] lg:gap-14">
          <ol className="relative space-y-8 border-l border-white/10 pl-6">
            {controlFlow.map((step, index) => (
              <li key={step.title} className="relative">
                <span className="absolute -left-[31px] flex h-6 w-6 items-center justify-center rounded-full border border-sky-400/30 bg-[#070c17] font-mono text-[10px] text-sky-300">
                  {index + 1}
                </span>
                <h3 className="text-base font-semibold text-foreground">{step.title}</h3>
                <p className="mt-1.5 text-sm leading-6 text-muted-foreground">{step.body}</p>
              </li>
            ))}
          </ol>
          <CodeBlock code={foundationWork} language="yaml" filename="manifestwork-excerpt.yaml" />
        </div>
      </Section>

      <Section>
        <div className="grid gap-12 lg:grid-cols-2 lg:items-start lg:gap-16">
          <div>
            <SectionHeading
              align="left"
              eyebrow="Truth and freshness"
              title="A Cell is Ready only when its observations are alive"
              description="KubeCell separates “the object exists” from “the system is healthy”. Lease freshness and inventory freshness are first-class conditions, because stale data is the most common cause of wrong capacity decisions."
            />
            <div className="mt-8 overflow-hidden rounded-2xl border border-white/10">
              <table className="w-full border-collapse text-left text-sm">
                <thead>
                  <tr className="bg-white/[0.03]">
                    <th className="px-4 py-3 font-medium text-muted-foreground">Signal</th>
                    <th className="px-4 py-3 font-medium text-muted-foreground">Means</th>
                    <th className="px-4 py-3 font-medium text-muted-foreground">Failure mode</th>
                  </tr>
                </thead>
                <tbody>
                  {freshnessChecks.map((row) => (
                    <tr key={row.check} className="border-t border-white/5">
                      <td className="px-4 py-3 font-mono text-xs text-sky-300">{row.check}</td>
                      <td className="px-4 py-3 text-muted-foreground">{row.meaning}</td>
                      <td className="px-4 py-3 text-muted-foreground">{row.staleRisk}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          <div className="space-y-6">
            <SectionHeading
              align="left"
              eyebrow="Endpoint discovery"
              title="Addresses are observed, never configured"
              description="The API address is not part of spec. The controller reads Ready host nodes through the proxy, takes the InternalIP of the first node by name, and records it in status."
            />
            <CodeBlock
              language="bash"
              filename="endpoint.sh"
              code={`$ kubectl -n kubecell-system get vc child-dev \\
    -o jsonpath='{.status.endpoint}'
{"address":"10.0.12.7","port":31234}

# When the address changes, kubeconfigs are republished
# automatically. Clients pick up the new file; no CR edit.`}
            />
            <p className="text-sm leading-6 text-muted-foreground">
              Exposure method, port range, and proxy identity are platform constants. That is why
              they are absent from the CRDs: there is exactly one safe value, so exposing them would
              only create drift.
            </p>
          </div>
        </div>
      </Section>

      <Section>
        <div className="grid gap-12 lg:grid-cols-[1fr_1fr] lg:items-center lg:gap-16">
          <div>
            <SectionHeading
              align="left"
              eyebrow="Code layout"
              title="Where to look in the repository"
              description="The design is small enough to hold in your head: four CRDs in api/, one controller in internal/, two charts, and an installer that owns the release bundle."
            />
            <p className="mt-6 text-sm leading-7 text-muted-foreground">
              Controllers never install host software; the installer does, at pinned versions. Charts
              never mutate OCM objects. Those two rules explain most of the directory structure.
            </p>
          </div>
          <RepoTree />
        </div>
      </Section>

      <Section className="border-y border-white/5 bg-white/[0.015]">
        <div className="grid gap-12 lg:grid-cols-[0.9fr_1.1fr] lg:items-center lg:gap-16">
          <div>
            <SectionHeading
              align="left"
              eyebrow="Built on upstream"
              title="Upstream does the work; KubeCell owns the contract"
              description="K3k, K3s, TopoLVM, Traefik, and OCM are used through their public APIs and never patched. You get upstream fixes and behavior directly, and every release can pin and verify the exact combination it was tested with."
            />
            <ul className="mt-8 space-y-3">
              {nonGoals.map((item) => (
                <li key={item} className="flex gap-2.5 text-sm leading-6 text-muted-foreground">
                  <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-violet-400/80" />
                  {item}
                </li>
              ))}
            </ul>
          </div>
          <OrbitingStack />
        </div>

        <div className="mt-16">
          <SectionHeading
            align="left"
            eyebrow="Locked combination"
            title="Versions that move together"
            description="Changing any of these requires a release, not a CR edit."
          />
          <div className="mt-8 overflow-hidden rounded-2xl border border-white/10">
            <div className="overflow-x-auto">
              <table className="w-full min-w-[640px] border-collapse text-left text-sm">
                <thead>
                  <tr className="bg-white/[0.03]">
                    <th className="px-5 py-3.5 font-medium text-muted-foreground">Component</th>
                    <th className="px-5 py-3.5 font-medium text-muted-foreground">Version</th>
                    <th className="px-5 py-3.5 font-medium text-muted-foreground">Notes</th>
                  </tr>
                </thead>
                <tbody>
                  {pinnedStack.map((row) => (
                    <tr key={row.component} className="border-t border-white/5">
                      <td className="px-5 py-3.5 font-medium text-foreground">{row.component}</td>
                      <td className="px-5 py-3.5 font-mono text-xs text-sky-300">{row.version}</td>
                      <td className="px-5 py-3.5 text-muted-foreground">{row.note}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </Section>

      <Section>
        <SectionHeading
          eyebrow="Ingress and north-south"
          title="One host ingress, predictable hostnames"
          description="Host Traefik owns ports 80 and 443 for the whole host. It watches only VirtualCluster namespaces that carry the KubeCell label, so one noisy tenant cannot claim the edge."
        />
        <div className="mt-10 grid gap-4 md:grid-cols-3">
          {[
            {
              title: "You choose the Ingress name",
              body: "Create a standard Ingress with class kubecell in the child cluster. The hostname derives as <ingress>.<vc>.<apps-suffix>; the suffix is a platform constant.",
            },
            {
              title: "Endpoints stay in sync",
              body: "The controller mirrors child Endpoints into the host namespace and strips port names that would break routing.",
            },
            {
              title: "TLS is an operator decision",
              body: "Certificates are issued at the host ingress. The design keeps v1 to HTTP(S) rather than promising protocols it cannot keep.",
            },
          ].map((card) => (
            <div key={card.title} className="rounded-2xl border border-white/10 bg-white/[0.02] p-6">
              <Layers className="h-5 w-5 text-sky-300" />
              <h3 className="mt-3 text-base font-semibold text-foreground">{card.title}</h3>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">{card.body}</p>
            </div>
          ))}
        </div>
        <div className="mt-10">
          <FailureModes />
        </div>
      </Section>
    </>
  );
}
