import type { Metadata } from "next";

import { DocsMobileNav, DocsSidebar } from "@/components/docs/docs-sidebar";
import { DocsPager } from "@/components/docs/docs-pager";

export const metadata: Metadata = {
  title: {
    default: "Documentation",
    template: "%s | KubeCell Docs",
  },
  description:
    "KubeCell documentation: concepts, installation, host onboarding, day-2 operations, API reference, security, and troubleshooting.",
  alternates: { canonical: "/docs/" },
};

export default function DocsLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="relative">
      <div className="pointer-events-none absolute inset-x-0 top-0 h-72 bg-grid-faint opacity-40" />
      <div className="container-page relative">
        <div className="grid gap-10 py-12 lg:grid-cols-[16rem_minmax(0,1fr)] lg:gap-14 lg:py-16">
          <aside className="lg:sticky lg:top-24 lg:h-[calc(100vh-8rem)] lg:overflow-y-auto lg:pr-2">
            <DocsMobileNav />
            <div className="hidden lg:block">
              <DocsSidebar />
            </div>
          </aside>
          <div className="min-w-0 max-w-3xl">
            {children}
            <DocsPager />
          </div>
        </div>
      </div>
    </div>
  );
}
