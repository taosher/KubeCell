import Link from "next/link";
import { ArrowRight, AlertTriangle, CheckCircle2 } from "lucide-react";

import { BlurFade } from "@/components/magicui/blur-fade";
import { CodeBlock } from "@/components/code-block";
import { JsonLd } from "@/components/json-ld";
import { PageHero, Section, SectionHeading } from "@/components/section";
import { Badge } from "@/components/ui/badge";
import { absoluteUrl } from "@/lib/site";
import { buildMetadata } from "@/lib/metadata";

export const metadata = buildMetadata({
  title: "Quickstart",
  description:
    "Go from a fresh host to a working developer cluster in a handful of commands: preflight, join, approve, prepare, then apply an eight-line manifest.",
  path: "/quickstart/",
  keywords: [
    "KubeCell quickstart",
    "install KubeCell",
    "VirtualCluster tutorial",
    "K3k shared mode",
    "host onboarding",
  ],
});

const steps = [
  {
    id: "prerequisites",
    step: "00",
    title: "What you need before you start",
    body: (
      <>
        <p>
          KubeCell needs one small management cluster and at least one physical host. Both run K3s and
          must be able to pull the images in your release bundle.
        </p>
        <ul className="mt-4 space-y-2 text-sm leading-6 text-muted-foreground">
          <li className="flex gap-2">
            <CheckCircle2 className="mt-1 h-4 w-4 shrink-0 text-emerald-400" />
            A management cluster (single-node K3s) with admin access and outbound network access.
          </li>
          <li className="flex gap-2">
            <CheckCircle2 className="mt-1 h-4 w-4 shrink-0 text-emerald-400" />
            One or more host machines prepared with the platform baseline: kernel drivers, firmware,
            LVM volume groups, and preloaded images.
          </li>
          <li className="flex gap-2">
            <CheckCircle2 className="mt-1 h-4 w-4 shrink-0 text-emerald-400" />
            A KubeCell release bundle from the releases page: the manifests and images for one tested
            version, with digests included.
          </li>
          <li className="flex gap-2">
            <CheckCircle2 className="mt-1 h-4 w-4 shrink-0 text-emerald-400" />
            A label on each host node describing what the machine offers, for example{" "}
            <code className="font-mono text-xs">hardware.kubecell.io/profile=ascend-910b</code>, plus
            the storage topology labels your platform team provides.
          </li>
        </ul>
        <div className="mt-5 flex gap-3 rounded-xl border border-amber-400/20 bg-amber-500/[0.06] p-4">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-300" />
          <p className="text-sm leading-6 text-amber-100/80">
            KubeCell is pre-release software. Run the quickstart in a lab environment, not in
            production.
          </p>
        </div>
      </>
    ),
    code: `# Point at the management cluster (hub) and the host cluster
export KUBECONFIG_HUB=~/.kube/hub.yaml
export KUBECONFIG_HOST=~/.kube/host.yaml

# Verify the pinned versions in the bundle before anything else
ls kubecell-release/manifests | head`,
    language: "bash" as const,
    filename: "prerequisites.sh",
  },
  {
    id: "doctor",
    step: "01",
    title: "Run the read-only preflight",
    body: (
      <p>
        <code className="font-mono text-xs">doctor</code> is read-only by default and stops at the
        first failure. Fix everything it reports before you continue; the installer refuses to guess.
      </p>
    ),
    code: `go run ./cmd/kubecell-installer doctor management --bundle ./kubecell-release
go run ./cmd/kubecell-installer doctor host \\
  --bundle ./kubecell-release \\
  --device huawei.com/Ascend910`,
    language: "bash" as const,
    filename: "preflight.sh",
  },
  {
    id: "management",
    step: "02",
    title: "Install the management plane",
    body: (
      <>
        <p>
          CRDs first, then the hub that connects your hosts, then the KubeCell controller. The
          installer stays read-only unless you pass both{" "}
          <code className="font-mono text-xs">--apply</code> and{" "}
          <code className="font-mono text-xs">--confirm</code>.
        </p>
        <CodeBlock
          className="mt-4"
          language="bash"
          filename="management.sh"
          code={`kubectl apply -f kubecell-release/manifests/crds

go run ./cmd/kubecell-installer management \\
  --bundle ./kubecell-release --apply --confirm`}
        />
      </>
    ),
    code: `# The management chart bundles the optional Kite dashboard
# (kite.enabled, on by default) for viewing Cell and VirtualCluster CRs.
helm list -n kubecell-system`,
    language: "bash" as const,
    filename: "verify-management.sh",
  },
  {
    id: "host",
    step: "03",
    title: "Onboard the host in four commands",
    body: (
      <p>
        Join, approve by hand, install the host chart, and generate the Cell from live discovery.
        The manual approval is a deliberate safety gate: the installer never auto-approves a
        ManagedCluster.
      </p>
    ),
    code: `# 1. join (run against the host cluster)
KUBECONFIG=$KUBECONFIG_HOST clusteradm join \\
  --hub-token "$(clusteradm get token)" \\
  --hub-apiserver https://hub.example.com:6443 \\
  --cluster-name cell1 --wait

# 2. accept (run against the management cluster)
KUBECONFIG=$KUBECONFIG_HUB clusteradm accept --clusters cell1 --wait

# 3. host chart
kubectl create ns kubecell-system
helm install kubecell-host ./charts/kubecell-host \\
  --namespace kubecell-system \\
  --set image.repository=<controller-repo> --set image.tag=<tag>

# 4. Cell, generated from live discovery
go run ./cmd/kubecell-installer host --step=cell --cell cell1 \\
  --kubeconfig $KUBECONFIG_HOST | grep -v '^#' > cell.yaml
kubectl apply -n kubecell-system -f cell.yaml`,
    language: "bash" as const,
    filename: "host-onboarding.sh",
  },
  {
    id: "class",
    step: "04",
    title: "Draft a quota tier",
    body: (
      <p>
        <code className="font-mono text-xs">suggest-class</code> derives a tier from observed
        allocatable capacity. Adjust the numbers down to the tenant envelope you are comfortable
        with, then apply.
      </p>
    ),
    code: `go run ./cmd/kubecell-installer suggest-class \\
  --cell-file cell.yaml --name ascend-910b

kubectl apply -f virtualnodeclass.yaml
kubectl -n kubecell-system get cell cell1 -o wide`,
    language: "bash" as const,
    filename: "class.sh",
  },
  {
    id: "virtualcluster",
    step: "05",
    title: "Plan, then create the child cluster",
    body: (
      <p>
        Create a VirtualClusterPlan with identical content first. Wait for{" "}
        <code className="font-mono text-xs">status.decision=Accepted</code>, then create the
        VirtualCluster. When it reaches <code className="font-mono text-xs">phase=Ready</code>, fetch
        the admin kubeconfig Secret.
      </p>
    ),
    code: `kubectl apply -f virtualcluster-plan.yaml
kubectl -n kubecell-system get vcp child-dev-plan \\
  -o jsonpath='{.status.decision}{"\\n"}'

kubectl apply -f virtualcluster.yaml
kubectl -n kubecell-system wait --for=condition=Ready \\
  vc/child-dev --timeout=20m

# Export the admin kubeconfig (mode 600)
kubectl -n kubecell-system get secret \\
  kubecell-child-dev-<uid>-admin-kubeconfig \\
  -o jsonpath='{.data.kubeconfig\\.yaml}' | base64 -d > child-dev.yaml
chmod 600 child-dev.yaml
kubectl --kubeconfig child-dev.yaml get nodes`,
    language: "bash" as const,
    filename: "virtualcluster.sh",
  },
];

const howToLd = {
  "@context": "https://schema.org",
  "@type": "HowTo",
  name: "Provision your first KubeCell VirtualCluster",
  description:
    "Install the KubeCell management plane, onboard a physical host as a Cell, and create an isolated Kubernetes child cluster.",
  url: absoluteUrl("/quickstart/"),
  step: steps.map((step, index) => ({
    "@type": "HowToStep",
    position: index + 1,
    name: step.title,
    url: absoluteUrl(`/quickstart/#${step.id}`),
  })),
};

export default function QuickstartPage() {
  return (
    <>
      <JsonLd data={howToLd} />
      <PageHero
        eyebrow="Quickstart"
        title="From a fresh host to your first cluster"
        description="Six steps, four of them commands. The first command only reads, so you can run the whole flow on a lab host without fear."
      >
        <Badge variant="outline" className="border-emerald-400/30 text-emerald-300">
          ~30 minutes hands-on, including image pulls
        </Badge>
        <Link
          href="/docs/installation/"
          className="inline-flex items-center gap-1.5 text-sm font-medium text-sky-400 transition-colors hover:text-sky-300"
        >
          Full installation guide
          <ArrowRight className="h-3.5 w-3.5" />
        </Link>
      </PageHero>

      <Section>
        <div className="space-y-16">
          {steps.map((step, index) => (
            <BlurFade key={step.id} delay={index * 0.04} inView>
              <div id={step.id} className="scroll-mt-24">
                <div className="grid gap-8 lg:grid-cols-[0.85fr_1.15fr] lg:gap-12">
                  <div>
                    <div className="flex items-center gap-3">
                      <span className="font-mono text-xs text-sky-400">{step.step}</span>
                      <span className="h-px flex-1 bg-gradient-to-r from-sky-400/40 to-transparent" />
                    </div>
                    <h2 className="mt-4 text-2xl font-semibold tracking-tight text-foreground">
                      {step.title}
                    </h2>
                    <div className="mt-4 text-sm leading-7 text-muted-foreground">{step.body}</div>
                  </div>
                  <CodeBlock code={step.code} language={step.language} filename={step.filename} />
                </div>
              </div>
            </BlurFade>
          ))}
        </div>
      </Section>

      <Section className="border-t border-white/5 bg-white/[0.015]">
        <SectionHeading
          eyebrow="Next steps"
          title="Now that a cluster exists"
          description="The interesting part is day two: keeping tenants honest, wiring ingress, and reclaiming storage."
        />
        <div className="mt-10 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {[
            {
              href: "/docs/concepts/",
              title: "Understand the model",
              description: "How hosts, quota tiers, and clusters fit together.",
            },
            {
              href: "/docs/operations/",
              title: "Operate it",
              description: "Capacity, credentials, quota changes, deletion.",
            },
            {
              href: "/docs/security/",
              title: "Audit isolation",
              description: "Quotas, network policy, and blocked workloads.",
            },
            {
              href: "/docs/troubleshooting/",
              title: "Debug failures",
              description: "Symptom-first fixes for the common cases.",
            },
          ].map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className="group rounded-xl border border-white/10 bg-white/[0.02] p-5 transition-colors hover:border-sky-400/30 hover:bg-white/[0.04]"
            >
              <h3 className="text-sm font-semibold text-foreground">{item.title}</h3>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">{item.description}</p>
              <span className="mt-3 inline-flex items-center gap-1 text-xs font-medium text-sky-400">
                Open
                <ArrowRight className="h-3 w-3 transition-transform group-hover:translate-x-0.5" />
              </span>
            </Link>
          ))}
        </div>
      </Section>
    </>
  );
}
