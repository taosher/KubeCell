"use client";

import {
  Boxes,
  Gauge,
  LayoutGrid,
  Lock,
  Pin,
  SlidersHorizontal,
} from "lucide-react";

import { AnimatedGridPattern } from "@/components/magicui/animated-grid-pattern";
import { BentoCard, BentoGrid } from "@/components/magicui/bento-grid";
import { DotPattern } from "@/components/magicui/dot-pattern";
import { FlickeringGrid } from "@/components/magicui/flickering-grid";
import { HexagonPattern } from "@/components/magicui/hexagon-pattern";
import { RetroGrid } from "@/components/magicui/retro-grid";
import { SectionHeading } from "@/components/section";
import { cn } from "@/lib/utils";

const cards = [
  {
    Icon: Boxes,
    name: "Real clusters, not namespaces",
    description:
      "Every developer gets their own API server, so they can install CRDs, operators, and webhooks without stepping on anyone else.",
    href: "/docs/concepts/",
    cta: "What you get",
    className: "lg:col-span-1 lg:row-start-1 lg:row-end-1",
    background: (
      <DotPattern
        width={20}
        height={20}
        cx={1}
        cy={1}
        cr={1}
        className={cn("[mask-image:radial-gradient(18rem_circle_at_center,white,transparent)]")}
      />
    ),
  },
  {
    Icon: Gauge,
    name: "Know it will fit before you create it",
    description:
      "A dry-run request tells you whether CPU, memory, GPUs, and storage are available — and exactly what is missing when they are not. Nothing is created until you are ready.",
    href: "/docs/concepts/",
    cta: "How the dry run works",
    className: "lg:col-span-2 lg:row-start-1 lg:row-end-2",
    background: (
      <RetroGrid className="opacity-40" cellSize={48} opacity={0.35} />
    ),
  },
  {
    Icon: SlidersHorizontal,
    name: "Grow by picking a bigger tier",
    description:
      "Each cluster presents one node sized by the quota tier you choose. Need more CPU, memory, or another GPU? Move to the next tier instead of learning a new scaling model.",
    href: "/docs/concepts/",
    cta: "Understand sizing",
    className: "lg:col-span-2 lg:row-start-2 lg:row-end-3",
    background: (
      <AnimatedGridPattern
        numSquares={24}
        maxOpacity={0.14}
        duration={4}
        className={cn(
          "absolute inset-0 [mask-image:radial-gradient(20rem_circle_at_center,white,transparent)]"
        )}
      />
    ),
  },
  {
    Icon: Lock,
    name: "Isolation you can audit",
    description:
      "Quotas, network policy, and blocked privileged workloads are enforced on the host — visible in the same Kubernetes objects you already know.",
    href: "/docs/security/",
    cta: "Security model",
    className: "lg:col-span-1 lg:row-start-2 lg:row-end-3",
    background: (
      <HexagonPattern
        radius={22}
        gap={6}
        strokeDasharray="2 3"
        className="absolute inset-0 opacity-40"
      />
    ),
  },
  {
    Icon: LayoutGrid,
    name: "Manifests you can read in a minute",
    description:
      "You write which host to use and how much CPU, memory, and GPU you need. Versions, ports, and security defaults are handled for you.",
    href: "/docs/first-virtualcluster/",
    cta: "Write your first cluster",
    className: "lg:col-span-1 lg:row-start-3 lg:row-end-3",
    background: (
      <FlickeringGrid
        squareSize={3}
        gridGap={6}
        color="#2f7fe0"
        maxOpacity={0.25}
        flickerChance={0.08}
        className="absolute inset-0 [mask-image:radial-gradient(12rem_circle_at_center,white,transparent)]"
      />
    ),
  },
  {
    Icon: Pin,
    name: "Tested versions, predictable upgrades",
    description:
      "KubeCell pins the versions it validates and reports drift in resource status, so upgrades are a deliberate step — not a surprise on a Monday morning.",
    href: "/architecture/",
    cta: "See what is pinned",
    className: "lg:col-span-2 lg:row-start-3 lg:row-end-3",
    background: (
      <HexagonPattern
        radius={30}
        gap={10}
        direction="vertical"
        className="absolute inset-0 opacity-30"
      />
    ),
  },
];

export function FeatureBento() {
  return (
    <section className="relative py-20 sm:py-24">
      <div className="container-page">
        <SectionHeading
          eyebrow="Why KubeCell"
          title="Platform-grade isolation without a platform rewrite"
          description="Four resources to learn, one management plane to operate, and the Kubernetes workflow your team already knows."
        />
        <div className="mt-12">
          <BentoGrid className="lg:auto-rows-[18rem] lg:grid-cols-3">
            {cards.map((card) => (
              <BentoCard key={card.name} {...card} />
            ))}
          </BentoGrid>
        </div>
      </div>
    </section>
  );
}
