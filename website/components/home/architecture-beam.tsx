"use client";

import { useRef } from "react";
import { Boxes, CloudCog, FileCode2, Layers, Server, Workflow } from "lucide-react";

import { AnimatedBeam } from "@/components/magicui/animated-beam";
import { cn } from "@/lib/utils";

type BeamNode = {
  icon: React.ElementType;
  title: string;
  subtitle: string;
};

type Variant = "simple" | "technical";

const nodes: Record<Variant, { left: BeamNode[]; middle: BeamNode[]; right: BeamNode[] }> = {
  simple: {
    left: [
      { icon: FileCode2, title: "Your manifests", subtitle: "host, quota, cluster" },
      { icon: CloudCog, title: "KubeCell", subtitle: "one management cluster" },
    ],
    middle: [
      { icon: Layers, title: "Host setup", subtitle: "namespace + quota" },
      { icon: Workflow, title: "Cluster creation", subtitle: "control plane + storage" },
    ],
    right: [
      { icon: Server, title: "Your host", subtitle: "runs the workloads" },
      { icon: Boxes, title: "Your cluster", subtitle: "kubectl + kubeconfig" },
    ],
  },
  technical: {
    left: [
      { icon: FileCode2, title: "KubeCell CRDs", subtitle: "Cell · Class · Plan · VC" },
      { icon: CloudCog, title: "OCM Hub", subtitle: "desired state only" },
    ],
    middle: [
      { icon: Layers, title: "Foundation Work", subtitle: "namespace + quota" },
      { icon: Workflow, title: "Instance Work", subtitle: "K3k shared-mode cluster" },
    ],
    right: [
      { icon: Server, title: "Cell host", subtitle: "work-agent applies" },
      { icon: Boxes, title: "VirtualCluster", subtitle: "child K3s API + kubelet" },
    ],
  },
};

function Node({
  nodeRef,
  icon: Icon,
  title,
  subtitle,
  className,
}: {
  nodeRef: React.RefObject<HTMLDivElement | null>;
  icon: React.ElementType;
  title: string;
  subtitle: string;
  className?: string;
}) {
  return (
    <div
      ref={nodeRef}
      className={cn(
        "z-10 w-full max-w-56 rounded-xl border border-white/10 bg-[#080d19]/90 px-4 py-3 shadow-[0_10px_40px_-20px_rgba(47,127,224,0.8)] backdrop-blur",
        className
      )}
    >
      <div className="flex items-center gap-2.5">
        <span className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-sky-400/20 bg-sky-500/10 text-sky-300">
          <Icon className="h-4 w-4" />
        </span>
        <div className="min-w-0">
          <p className="truncate text-sm font-medium text-foreground">{title}</p>
          <p className="truncate text-xs text-muted-foreground">{subtitle}</p>
        </div>
      </div>
    </div>
  );
}

export function ArchitectureBeam({
  className,
  variant = "simple",
}: {
  className?: string;
  variant?: Variant;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const crsRef = useRef<HTMLDivElement>(null);
  const hubRef = useRef<HTMLDivElement>(null);
  const foundationRef = useRef<HTMLDivElement>(null);
  const instanceRef = useRef<HTMLDivElement>(null);
  const hostRef = useRef<HTMLDivElement>(null);
  const childRef = useRef<HTMLDivElement>(null);

  const set = nodes[variant];

  return (
    <div className={cn("w-full", className)}>
      <div className="flex flex-col gap-3 md:hidden">
        {[
          ...set.left,
          { icon: Layers, title: "Host setup + cluster creation", subtitle: "applied on the host" },
          ...set.right,
        ].map((node, index, list) => (
          <div key={node.title}>
            <div className="rounded-xl border border-white/10 bg-[#080d19]/90 px-4 py-3">
              <div className="flex items-center gap-2.5">
                <span className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-sky-400/20 bg-sky-500/10 text-sky-300">
                  <node.icon className="h-4 w-4" />
                </span>
                <div>
                  <p className="text-sm font-medium text-foreground">{node.title}</p>
                  <p className="text-xs text-muted-foreground">{node.subtitle}</p>
                </div>
              </div>
            </div>
            {index < list.length - 1 ? (
              <div className="mx-auto h-5 w-px bg-gradient-to-b from-sky-400/60 to-transparent" />
            ) : null}
          </div>
        ))}
      </div>

      <div
        ref={containerRef}
        className="relative hidden w-full items-stretch justify-between gap-4 overflow-hidden rounded-2xl border border-white/5 bg-white/[0.015] px-6 py-10 sm:px-10 md:flex"
      >
        <div className="pointer-events-none absolute inset-0 bg-grid-faint opacity-40" />
        <div className="relative z-10 flex flex-col justify-center gap-6">
          <Node nodeRef={crsRef} {...set.left[0]} />
          <Node nodeRef={hubRef} {...set.left[1]} />
        </div>

        <div className="relative z-10 flex flex-col justify-center gap-16">
          <Node nodeRef={foundationRef} {...set.middle[0]} />
          <Node nodeRef={instanceRef} {...set.middle[1]} />
        </div>

        <div className="relative z-10 flex flex-col justify-center gap-6">
          <Node nodeRef={hostRef} {...set.right[0]} />
          <Node nodeRef={childRef} {...set.right[1]} />
        </div>

        <AnimatedBeam
          containerRef={containerRef}
          fromRef={crsRef}
          toRef={hubRef}
          duration={4}
          gradientStartColor="#38bdf8"
          gradientStopColor="#2f7fe0"
          pathColor="#2f7fe0"
          pathOpacity={0.15}
        />
        <AnimatedBeam
          containerRef={containerRef}
          fromRef={hubRef}
          toRef={foundationRef}
          duration={5}
          delay={0.4}
          gradientStartColor="#2f7fe0"
          gradientStopColor="#8b5cf6"
          pathColor="#2f7fe0"
          pathOpacity={0.15}
        />
        <AnimatedBeam
          containerRef={containerRef}
          fromRef={hubRef}
          toRef={instanceRef}
          duration={5}
          delay={0.9}
          curvature={-60}
          gradientStartColor="#2f7fe0"
          gradientStopColor="#8b5cf6"
          pathColor="#2f7fe0"
          pathOpacity={0.15}
        />
        <AnimatedBeam
          containerRef={containerRef}
          fromRef={foundationRef}
          toRef={hostRef}
          duration={4}
          delay={0.6}
          gradientStartColor="#38bdf8"
          gradientStopColor="#22d3ee"
          pathColor="#2f7fe0"
          pathOpacity={0.15}
        />
        <AnimatedBeam
          containerRef={containerRef}
          fromRef={instanceRef}
          toRef={childRef}
          duration={4}
          delay={1.1}
          curvature={60}
          gradientStartColor="#8b5cf6"
          gradientStopColor="#38bdf8"
          pathColor="#2f7fe0"
          pathOpacity={0.15}
        />
        <AnimatedBeam
          containerRef={containerRef}
          fromRef={hostRef}
          toRef={childRef}
          duration={3.5}
          delay={1.4}
          gradientStartColor="#22d3ee"
          gradientStopColor="#2f7fe0"
          pathColor="#2f7fe0"
          pathOpacity={0.15}
        />
      </div>
    </div>
  );
}
