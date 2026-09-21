import Link from "next/link";
import { ArrowRight, BrainCircuit, GraduationCap, Layers3, ShipWheel, TestTube2 } from "lucide-react";

import { GlareHover } from "@/components/magicui/glare-hover";
import { SectionHeading } from "@/components/section";

const useCases = [
  {
    icon: BrainCircuit,
    title: "Accelerator sandboxes",
    description:
      "Give each researcher their own cluster with two dedicated Ascend 910B cards — no partitioning tricks, no shared queue.",
    href: "/use-cases/#accelerator-sandboxes",
  },
  {
    icon: TestTube2,
    title: "Ephemeral test environments",
    description:
      "Create a cluster per pull request or integration run, hand over the kubeconfig, then delete it and let KubeCell clean up the host.",
    href: "/use-cases/#ephemeral-environments",
  },
  {
    icon: GraduationCap,
    title: "Workshops and training",
    description:
      "Forty students, one physical host, forty clusters. Credentials are easy to hand out, and quotas stop one busy cluster from ruining the class.",
    href: "/use-cases/#training-labs",
  },
  {
    icon: ShipWheel,
    title: "Edge and field labs",
    description:
      "Hosts live where the hardware is. They connect outbound to one management plane, so there is no inbound tunnel to maintain per site.",
    href: "/use-cases/#field-labs",
  },
  {
    icon: Layers3,
    title: "Self-service for platform teams",
    description:
      "Publish hosts and quota tiers once; teams self-serve clusters inside the limits you defined, with capacity visible to everyone.",
    href: "/use-cases/#platform-landlord",
  },
];

export function UseCasePreview() {
  return (
    <section className="relative py-20 sm:py-24">
      <div className="container-page">
        <SectionHeading
          eyebrow="Use cases"
          title="One operator, five shapes of shared hardware"
          description="If a team needs a real Kubernetes API and the hardware lives on shared bare metal, KubeCell is the seam between them."
        />

        <div className="mt-12 grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {useCases.map((useCase) => (
            <GlareHover
              key={useCase.title}
              className="h-full rounded-xl border border-white/10"
              background="rgba(255,255,255,0.02)"
              color="#9ad4ff"
              opacity={0.35}
              duration={600}
            >
              <div className="flex h-full flex-col p-6">
                <span className="inline-flex h-10 w-10 items-center justify-center rounded-xl border border-sky-400/20 bg-sky-500/10 text-sky-300">
                  <useCase.icon className="h-5 w-5" />
                </span>
                <h3 className="mt-4 text-base font-semibold text-foreground">{useCase.title}</h3>
                <p className="mt-2 flex-1 text-sm leading-6 text-muted-foreground">
                  {useCase.description}
                </p>
                <Link
                  href={useCase.href}
                  className="mt-4 inline-flex items-center gap-1.5 text-sm font-medium text-sky-400 transition-colors hover:text-sky-300"
                >
                  Read the scenario
                  <ArrowRight className="h-3.5 w-3.5" />
                </Link>
              </div>
            </GlareHover>
          ))}
        </div>
      </div>
    </section>
  );
}
