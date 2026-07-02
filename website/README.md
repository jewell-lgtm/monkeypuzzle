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

```bash
pnpm test    # vitest — waitlist handler branches
pnpm smoke   # build first; serves dist/ and checks every route
```

(The old nav/CTA links to the mp-server login were removed on purpose — no
dead ends until the hosted dashboard launches.)

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
