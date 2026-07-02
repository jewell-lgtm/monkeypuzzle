// @ts-check
import { defineConfig } from 'astro/config';
import node from '@astrojs/node';

// https://astro.build/config
//
// The site is prerendered (static) except for Astro Actions — the email
// capture form posts to a server-rendered action endpoint, which needs the
// node adapter. Standalone mode: `node dist/server/entry.mjs` serves both the
// prerendered pages and the action route (see Dockerfile).
export default defineConfig({
  adapter: node({ mode: 'standalone' }),
  security: {
    // Astro ≥5.18: with this list EMPTY, every request's host collapses to
    // "localhost", which makes the CSRF origin check 403 ALL form posts.
    // List every host the site is actually served on.
    allowedDomains: [
      { hostname: 'monkeypuzzle.dev' },
      { hostname: '**.monkeypuzzle.dev' },
      { hostname: 'localhost' },
      { hostname: '127.0.0.1' },
    ],
  },
});
