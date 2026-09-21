import Link from "next/link";
import { Github } from "lucide-react";

import { Logo } from "@/components/logo";
import { docsFlat } from "@/lib/docs-nav";
import { primaryNav, siteConfig } from "@/lib/site";

const projectLinks = [
  { title: "GitHub repository", href: siteConfig.github },
  { title: "Technical design", href: siteConfig.docs.design },
  { title: "Host onboarding guide", href: siteConfig.docs.onboarding },
  { title: "CI plan", href: siteConfig.docs.ciPlan },
  { title: "Issues", href: siteConfig.githubIssues },
];

export function SiteFooter() {
  return (
    <footer className="border-t border-white/5 bg-[#04060d]">
      <div className="container-page py-14">
        <div className="grid gap-10 lg:grid-cols-[1.4fr_1fr_1fr_1fr]">
          <div className="max-w-sm">
            <Logo />
            <p className="mt-4 text-sm leading-6 text-muted-foreground">
              Give every developer their own isolated Kubernetes cluster on the hardware you already
              own — with hard CPU, memory, and GPU quotas, persistent storage, and standard kubectl.
            </p>
            <p className="mt-4 text-xs leading-5 text-amber-300/80">
              KubeCell is in active development and not yet stable. Do not use it for production
              workloads.
            </p>
          </div>

          <div>
            <h3 className="text-sm font-semibold text-foreground">Product</h3>
            <ul className="mt-4 space-y-2.5 text-sm">
              {primaryNav.map((item) => (
                <li key={item.href}>
                  <Link
                    href={item.href}
                    className="text-muted-foreground transition-colors hover:text-foreground"
                  >
                    {item.title}
                  </Link>
                </li>
              ))}
              <li>
                <Link
                  href="/docs/faq/"
                  className="text-muted-foreground transition-colors hover:text-foreground"
                >
                  FAQ
                </Link>
              </li>
            </ul>
          </div>

          <div>
            <h3 className="text-sm font-semibold text-foreground">Documentation</h3>
            <ul className="mt-4 space-y-2.5 text-sm">
              {docsFlat.slice(0, 6).map((item) => (
                <li key={item.href}>
                  <Link
                    href={item.href}
                    className="text-muted-foreground transition-colors hover:text-foreground"
                  >
                    {item.title}
                  </Link>
                </li>
              ))}
            </ul>
          </div>

          <div>
            <h3 className="text-sm font-semibold text-foreground">Project</h3>
            <ul className="mt-4 space-y-2.5 text-sm">
              {projectLinks.map((item) => (
                <li key={item.href}>
                  <a
                    href={item.href}
                    target="_blank"
                    rel="noreferrer"
                    className="inline-flex items-center gap-1.5 text-muted-foreground transition-colors hover:text-foreground"
                  >
                    {item.title}
                    <Github className="h-3.5 w-3.5 opacity-60" />
                  </a>
                </li>
              ))}
            </ul>
          </div>
        </div>

        <div className="mt-12 flex flex-col gap-3 border-t border-white/5 pt-6 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
          <p>© {new Date().getFullYear()} KubeCell contributors. Built in the open on GitHub.</p>
          <p className="font-mono">
            Open source · pre-release · built on Kubernetes
          </p>
        </div>
      </div>
    </footer>
  );
}
