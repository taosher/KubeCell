import type { ReactNode } from "react";
import { AlertTriangle, Info, Lightbulb } from "lucide-react";

import { cn } from "@/lib/utils";

const styles = {
  info: {
    wrapper: "border-sky-400/20 bg-sky-500/[0.06]",
    icon: "text-sky-300",
    Icon: Info,
  },
  warning: {
    wrapper: "border-amber-400/20 bg-amber-500/[0.06]",
    icon: "text-amber-300",
    Icon: AlertTriangle,
  },
  tip: {
    wrapper: "border-emerald-400/20 bg-emerald-500/[0.06]",
    icon: "text-emerald-300",
    Icon: Lightbulb,
  },
} as const;

export function Callout({
  variant = "info",
  title,
  children,
  className,
}: {
  variant?: keyof typeof styles;
  title?: string;
  children: ReactNode;
  className?: string;
}) {
  const style = styles[variant];
  const Icon = style.Icon;
  return (
    <div className={cn("my-6 flex gap-3 rounded-xl border p-4", style.wrapper, className)}>
      <Icon className={cn("mt-0.5 h-4 w-4 shrink-0", style.icon)} />
      <div className="text-sm leading-6 text-foreground/85">
        {title ? <p className="mb-1 font-semibold text-foreground">{title}</p> : null}
        {children}
      </div>
    </div>
  );
}

export function Prose({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn("prose-kubecell", className)}>{children}</div>;
}
