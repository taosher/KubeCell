# KubeCell Website

The English marketing site and documentation for KubeCell, built with
[vinext](https://github.com/cloudflare/vinext) (Next.js API surface on Vite),
[shadcn/ui](https://ui.shadcn.com), and [Magic UI](https://magicui.design), deployed to
**Cloudflare Pages** with [Wrangler](https://developers.cloudflare.com/workers/wrangler/).

## Stack

| Concern | Choice |
| --- | --- |
| Framework | `vinext` (App Router, React Server Components) on Vite 8 |
| Output | Static export (`output: "export"`, `trailingSlash: true`) written to `dist/client` |
| Styling | Tailwind CSS v4 + shadcn/ui theme tokens |
| Components | shadcn/ui (`components/ui`) and Magic UI (`components/magicui`) |
| Deploy target | Cloudflare Pages via `wrangler pages deploy` |

The site is a pure static export: no Worker entry point, no request-time rendering. That keeps
SEO simple (real HTML per route) and hosting cheap.

## Local development

```sh
pnpm install
pnpm dev        # vinext dev server, http://localhost:3001
pnpm build      # static export to dist/client
pnpm preview    # serve the built export locally with wrangler pages dev
pnpm typecheck  # tsc --noEmit
```

## Deploying to Cloudflare Pages

Create the Pages project once (the name must match `deploy/wrangler.jsonc`):

```sh
pnpm exec wrangler pages project create kubecell --production-branch=main
pnpm build
pnpm deploy     # wrangler pages deploy --cwd deploy
```

`deploy/wrangler.jsonc` is intentionally outside the project root: vinext treats a root
`wrangler.jsonc` as a Workers project and would require the Cloudflare Vite plugin. Wrangler Pages
does not accept a custom config path, so the deploy script runs Wrangler with `--cwd deploy`.

Authentication uses `wrangler login` locally or `CLOUDFLARE_API_TOKEN` plus
`CLOUDFLARE_ACCOUNT_ID` in CI.

### GitHub Actions

`.github/workflows/website.yml` builds the site and deploys it on every push to `main` that touches
`website/**`. Configure these repository secrets:

| Secret | Purpose |
| --- | --- |
| `CLOUDFLARE_API_TOKEN` | Token with the *Edit Cloudflare Workers* template permissions |
| `CLOUDFLARE_ACCOUNT_ID` | Cloudflare account ID that owns the Pages project |

Optional repository variable:

| Variable | Purpose |
| --- | --- |
| `SITE_URL` | Canonical site URL, passed to the build as `NEXT_PUBLIC_SITE_URL` |

## Project layout

```text
app/                    routes, metadata, sitemap, robots, manifest
  docs/                 documentation pages with sidebar layout
  quickstart/           quickstart guide
  use-cases/            scenario walkthroughs
  architecture/         technical principles
components/ui/          shadcn/ui primitives
components/magicui/     Magic UI components
components/home/        landing page sections
components/docs/        documentation shell
lib/                    site config, docs nav, content, syntax highlighting
public/                 logo, favicon/apple icon, OG image, llms.txt
deploy/wrangler.jsonc   Cloudflare Pages configuration
```

## Content and SEO

- Every page exports metadata (title, description, canonical URL, Open Graph, Twitter card).
- `app/sitemap.ts`, `app/robots.ts`, and `app/manifest.ts` generate the crawler files at build time.
- JSON-LD is emitted for Organization/WebSite (root layout), SoftwareSourceCode and HowTo
  (quickstart), ItemList (use cases), TechArticle (architecture), and FAQPage (FAQ).
- `public/og.png` is generated from `public/og.svg` with `rsvg-convert`.
- `public/llms.txt` summarizes the project for AI crawlers.

## Component registries

`components.json` maps two registries because the upstream defaults are not always reachable from
CI or restricted networks:

- `@s` → shadcn/ui `new-york` items from the shadcn-ui repository.
- `@magicui` → Magic UI registry items from the magicui repository.

Example:

```sh
REGISTRY_URL=https://raw.githubusercontent.com/shadcn-ui/ui/main/apps/v4/public/r \
  pnpm dlx shadcn@latest add @magicui/marquee
```
