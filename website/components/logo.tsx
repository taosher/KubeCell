import { cn } from "@/lib/utils";

export function Logo({
  className,
  withWordmark = true,
}: {
  className?: string;
  withWordmark?: boolean;
}) {
  return (
    <span className={cn("inline-flex items-center gap-2.5", className)}>
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img
        src="/kubecell-logo.svg"
        alt="KubeCell logo"
        width={36}
        height={39}
        className="h-9 w-auto"
      />
      {withWordmark ? (
        <span className="text-[17px] font-semibold tracking-tight text-foreground">
          Kube<span className="text-sky-400">Cell</span>
        </span>
      ) : null}
    </span>
  );
}
