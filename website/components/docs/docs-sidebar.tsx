"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

import { docsNav } from "@/lib/docs-nav";
import { cn } from "@/lib/utils";

export function DocsSidebar() {
  const pathname = usePathname();

  return (
    <nav aria-label="Documentation" className="space-y-8">
      {docsNav.map((group) => (
        <div key={group.label}>
          <p className="mb-3 font-mono text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
            {group.label}
          </p>
          <ul className="space-y-1 border-l border-white/10">
            {group.items.map((item) => {
              const active = pathname === item.href;
              return (
                <li key={item.href}>
                  <Link
                    href={item.href}
                    className={cn(
                      "-ml-px block border-l border-transparent py-1.5 pl-4 text-sm transition-colors",
                      active
                        ? "border-sky-400 font-medium text-foreground"
                        : "text-muted-foreground hover:border-white/20 hover:text-foreground"
                    )}
                  >
                    {item.title}
                  </Link>
                </li>
              );
            })}
          </ul>
        </div>
      ))}
    </nav>
  );
}

export function DocsMobileNav() {
  const pathname = usePathname();
  const current = docsNav.flatMap((group) => group.items).find((item) => item.href === pathname);

  return (
    <details className="group rounded-xl border border-white/10 bg-white/[0.02] lg:hidden">
      <summary className="flex cursor-pointer items-center justify-between px-4 py-3 text-sm font-medium text-foreground">
        <span>
          Docs menu
          {current ? <span className="text-muted-foreground"> · {current.title}</span> : null}
        </span>
        <span className="text-muted-foreground transition-transform group-open:rotate-45">+</span>
      </summary>
      <div className="border-t border-white/5 px-4 py-4">
        {docsNav.map((group) => (
          <div key={group.label} className="mb-4 last:mb-0">
            <p className="mb-2 font-mono text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
              {group.label}
            </p>
            <ul className="space-y-1">
              {group.items.map((item) => (
                <li key={item.href}>
                  <Link
                    href={item.href}
                    className={cn(
                      "block rounded-md px-2 py-1.5 text-sm",
                      pathname === item.href
                        ? "bg-white/5 font-medium text-foreground"
                        : "text-muted-foreground"
                    )}
                  >
                    {item.title}
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
    </details>
  );
}
