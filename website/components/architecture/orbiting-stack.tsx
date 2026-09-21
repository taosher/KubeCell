"use client";

import { Boxes, BoxesIcon, Cpu, Database, Gauge, HardDrive, Network, Route } from "lucide-react";

import { OrbitingCircles } from "@/components/magicui/orbiting-circles";

const inner = [
  { label: "K3k", icon: BoxesIcon },
  { label: "TopoLVM", icon: HardDrive },
  { label: "Traefik", icon: Route },
  { label: "Device Plugin", icon: Cpu },
];

const outer = [
  { label: "K3s", icon: Boxes },
  { label: "OCM", icon: Network },
  { label: "CNI / CoreDNS", icon: Network },
  { label: "LVM", icon: Database },
  { label: "RuntimeClass", icon: Gauge },
];

function Pill({ label, icon: Icon }: { label: string; icon: React.ElementType }) {
  return (
    <span className="inline-flex items-center gap-1.5 whitespace-nowrap rounded-full border border-white/10 bg-[#080d19] px-3 py-1.5 text-xs text-foreground shadow-[0_6px_24px_-12px_rgba(47,127,224,0.9)]">
      <Icon className="h-3.5 w-3.5 text-sky-300" />
      {label}
    </span>
  );
}

export function OrbitingStack() {
  return (
    <div className="relative flex h-[30rem] w-full items-center justify-center overflow-hidden rounded-2xl border border-white/5 bg-white/[0.015]">
      <div className="pointer-events-none absolute inset-0 bg-grid-faint opacity-30" />
      <div className="pointer-events-none absolute h-72 w-72 rounded-full bg-sky-500/10 blur-[80px]" />

      <div className="z-10 flex h-24 w-24 flex-col items-center justify-center gap-1 rounded-2xl border border-sky-400/30 bg-[#070c17] text-center shadow-[0_0_60px_-20px_rgba(47,127,224,0.9)]">
        <span className="font-mono text-[10px] uppercase tracking-widest text-sky-400">Cell host</span>
        <span className="text-sm font-semibold text-foreground">pinned baseline</span>
      </div>

      <OrbitingCircles radius={132} duration={24} iconSize={30} path={false}>
        {inner.map((item) => (
          <Pill key={item.label} {...item} />
        ))}
      </OrbitingCircles>

      <OrbitingCircles radius={208} duration={38} reverse iconSize={30} path={false}>
        {outer.map((item) => (
          <Pill key={item.label} {...item} />
        ))}
      </OrbitingCircles>
    </div>
  );
}
