# monkeypuzzle — website

The Monkeypuzzle marketing site. An [Astro](https://astro.build) static site that
implements the **Monkeypuzzle Design System** (claude.ai/design) — its tokens,
component primitives, and the `marketing` UI kit.

## Develop

```bash
pnpm install
pnpm dev        # http://localhost:4321
pnpm build      # static output → dist/
pnpm preview    # serve the built site
```

## Email capture (Astro Action)

The CTA band's form posts to the `subscribe` Astro Action (`src/actions/`),
which appends normalized emails to a JSONL file — `WAITLIST_FILE`, default
`./data/waitlist.jsonl`, mounted as a volume in production (k8s PVC at
`/data`). Core logic + tests live in `src/lib/waitlist.ts`. Because of the
action the site runs on the node adapter (`node dist/server/entry.mjs`);
pages are still fully prerendered. `security.allowedDomains` in
`astro.config.mjs` must list every host the site serves on, or Astro's CSRF
origin check rejects all form posts (Astro ≥5.18 behavior).

**Bot gate:** two layers in front of the sink — a honeypot field (`website`;
filled = fake success, nothing stored) and Cloudflare Turnstile. The widget's
site key is committed in `src/config.ts` (public by design; currently CF's
always-pass TEST key — swap in the real one). The secret key comes from the
`TURNSTILE_SECRET_KEY` env (k8s Secret `monkeypuzzle-website-turnstile`);
when it's set, verification **fails closed** — missing/rejected tokens and
Cloudflare outages all surface a visible retryable error. Unset (local dev,
tests), the gate is off.

```bash
pnpm test    # vitest — waitlist + turnstile branches
pnpm smoke   # build first; serves dist/ and checks every route
```

(The old nav/CTA links to the mp-server login were removed on purpose — no
dead ends until the hosted dashboard launches.)

## Docs section (`/docs`)

The repo's `docs/*.md` are published at `/docs/<name>/`, rendered through an
Astro content collection (`src/content.config.ts`, pages in
`src/pages/docs/`). The markdown is **vendored** into `src/content/docs/` by
`pnpm sync-docs` (`scripts/sync-docs.mjs`) because the Docker build context
is `website/` alone and cannot see `../docs`. The sync lifts each doc's H1
into frontmatter and rewrites links — sibling docs become site routes,
anything outside `docs/` becomes a GitHub link — so nothing dead-ends. Edit
the source docs, run `pnpm sync-docs`, commit both; the
`src/lib/docs-sync.test.ts` freshness test fails on drift.

## Design system

Everything visual derives from the design system, vendored here so the site is
self-contained:

- `src/styles/tokens/*.css` — the `--mp-*` design tokens (colors, type, spacing,
  radii, shadows, motion, base). Fonts: Space Grotesk + JetBrains Mono.
- `src/styles/components.css` — component primitives (`.mp-btn`, `.mp-card`,
  `.mp-badge`, `.mp-term`, `.mp-logo`), ported from the DS React components.
- `src/styles/site.css` — marketing page layout (ported from the kit's `kit.css`).
- `src/components/*.astro` — `Logo`, `Button`, `Badge`, `Card`, `TerminalBlock`,
  `Icon` (Lucide), plus the page sections (`Nav`, `Hero`, `Features`, `Steps`,
  `CtaBand`, `Footer`).

To re-sync against the source design system, use the `/design-sync` skill.
