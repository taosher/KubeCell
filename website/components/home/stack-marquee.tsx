import { Marquee } from "@/components/magicui/marquee";

const stack = [
  "Kubernetes",
  "K3s",
  "kubectl",
  "Helm",
  "Traefik",
  "CoreDNS",
  "TopoLVM",
  "LVM",
  "NVIDIA",
  "Ascend 910B",
  "ARM64",
  "x86_64",
];

export function StackMarquee() {
  return (
    <div className="relative border-y border-white/5 bg-white/[0.015] py-8">
      <p className="mb-5 text-center font-mono text-xs uppercase tracking-[0.24em] text-muted-foreground">
        Standard Kubernetes tooling, no new workflow to learn
      </p>
      <div className="relative">
        <Marquee pauseOnHover className="[--duration:45s] [--gap:2.5rem]">
          {stack.map((name) => (
            <span
              key={name}
              className="rounded-full border border-white/10 bg-white/[0.03] px-4 py-1.5 text-sm text-muted-foreground"
            >
              {name}
            </span>
          ))}
        </Marquee>
        <div className="pointer-events-none absolute inset-y-0 left-0 w-24 bg-gradient-to-r from-background to-transparent" />
        <div className="pointer-events-none inset-y-0 right-0 absolute w-24 bg-gradient-to-l from-background to-transparent" />
      </div>
    </div>
  );
}
