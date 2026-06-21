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

## Linking to the server app

The nav "Sign in" / "Get started" and the CTA buttons link to the **mp-server**
login (`GET /login` → WorkOS OAuth). The target is configurable in
`src/config.ts` via `SERVER_URL`, defaulting to the server's local dev address:

```bash
# default: http://localhost:8080 (mp-server PORT)
PUBLIC_SERVER_URL=https://app.monkeypuzzle.dev pnpm build
```

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
