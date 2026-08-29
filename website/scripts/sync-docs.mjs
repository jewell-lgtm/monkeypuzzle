// Vendors the repo's docs/ into src/content/docs/ for the /docs pages.
//
// The Docker build context is website/ alone (see Dockerfile), so the site
// cannot read ../docs at build time — the docs are committed here, same
// "self-contained" philosophy as the vendored design system. Re-run with
// `pnpm sync-docs` after editing docs/; the vitest freshness test fails when
// the vendored copies drift from the source.
//
// The transform is deterministic (no timestamps): same input, same output.
import { readdirSync, readFileSync, writeFileSync, mkdirSync, rmSync } from 'node:fs';
import { dirname, join, posix } from 'node:path';
import { fileURLToPath } from 'node:url';

// Kept in sync with GITHUB_URL in src/config.ts (a .mjs script can't import
// the TS module without a loader).
const GITHUB_BLOB = 'https://github.com/jewell-lgtm/monkeypuzzle/blob/main';

// Curated sidebar order; anything not listed sorts after, alphabetically.
const ORDER = [
  'getting-started',
  'workflow',
  'commands',
  'remote-development',
  'self-hosting',
  'architecture',
  'reviewing',
  'docker-development',
  'contributing',
];

// Rewrites one markdown link target. Repo-relative .md links inside docs/
// become site routes; anything resolving outside docs/ points at GitHub so
// no link on the site dead-ends.
export function rewriteTarget(target) {
  if (/^(https?:|mailto:|#)/.test(target)) return target;
  const [path, anchor = ''] = target.split('#');
  const hash = anchor ? `#${anchor}` : '';
  const resolved = posix.normalize(posix.join('docs', path));
  if (resolved.startsWith('docs/') && resolved.endsWith('.md')) {
    return `/docs/${resolved.slice('docs/'.length, -'.md'.length)}/${hash}`;
  }
  return `${GITHUB_BLOB}/${resolved}${hash}`;
}

// Transforms one source doc into its vendored form: title lifted out of the
// leading H1 into frontmatter (the page template renders it), inline link
// targets rewritten. Fenced code blocks are left untouched.
export function transformDoc(name, source) {
  const slug = name.replace(/\.md$/, '');
  const lines = source.split('\n');

  let title = slug;
  const firstHeading = lines.findIndex((l) => l.startsWith('# '));
  if (firstHeading !== -1) {
    title = lines[firstHeading].slice(2).trim();
    lines.splice(firstHeading, 1);
  }

  let inFence = false;
  const body = lines
    .map((line) => {
      if (/^\s*(```|~~~)/.test(line)) inFence = !inFence;
      if (inFence) return line;
      return line.replace(/\]\(([^)\s]+)\)/g, (_, target) => `](${rewriteTarget(target)})`);
    })
    .join('\n')
    .replace(/^\n+/, '');

  const order = ORDER.indexOf(slug);
  const frontmatter = [
    '---',
    `title: ${JSON.stringify(title)}`,
    `order: ${order === -1 ? ORDER.length : order}`,
    '---',
    `<!-- Generated from docs/${name} by scripts/sync-docs.mjs — edit the source, then run \`pnpm sync-docs\`. -->`,
    '',
  ].join('\n');
  return { slug, contents: frontmatter + body };
}

// Builds every vendored entry from a source docs directory.
export function buildDocEntries(sourceDir) {
  return readdirSync(sourceDir)
    .filter((f) => f.endsWith('.md'))
    .sort()
    .map((f) => transformDoc(f, readFileSync(join(sourceDir, f), 'utf8')));
}

const selfPath = fileURLToPath(import.meta.url);
if (process.argv[1] === selfPath) {
  const websiteRoot = dirname(dirname(selfPath));
  const sourceDir = join(websiteRoot, '..', 'docs');
  const destDir = join(websiteRoot, 'src', 'content', 'docs');
  rmSync(destDir, { recursive: true, force: true });
  mkdirSync(destDir, { recursive: true });
  const entries = buildDocEntries(sourceDir);
  for (const { slug, contents } of entries) {
    writeFileSync(join(destDir, `${slug}.md`), contents);
  }
  console.log(`synced ${entries.length} docs → src/content/docs/`);
}
