// Freshness guard for the vendored docs: src/content/docs/ must be exactly
// what scripts/sync-docs.mjs produces from the repo's docs/. Runs in the
// repo checkout (where ../docs exists) — the Docker build, which can't see
// ../docs, never runs tests.
import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
// eslint-disable-next-line import/no-relative-packages
import { buildDocEntries, rewriteTarget } from '../../scripts/sync-docs.mjs';

const websiteRoot = join(__dirname, '..', '..');
const sourceDir = join(websiteRoot, '..', 'docs');
const vendoredDir = join(websiteRoot, 'src', 'content', 'docs');

describe('sync-docs link rewriting', () => {
  it('maps sibling doc links to site routes, anchors preserved', () => {
    expect(rewriteTarget('commands.md')).toBe('/docs/commands/');
    expect(rewriteTarget('./workflow.md#sessions-are-interactive-only')).toBe(
      '/docs/workflow/#sessions-are-interactive-only',
    );
  });

  it('points repo files outside docs/ at GitHub', () => {
    expect(rewriteTarget('../apps/tmux/README.md')).toBe(
      'https://github.com/jewell-lgtm/monkeypuzzle/blob/main/apps/tmux/README.md',
    );
    expect(rewriteTarget('../README.md')).toBe(
      'https://github.com/jewell-lgtm/monkeypuzzle/blob/main/README.md',
    );
  });

  it('leaves absolute URLs and pure anchors alone', () => {
    expect(rewriteTarget('https://example.com/x.md')).toBe('https://example.com/x.md');
    expect(rewriteTarget('#some-heading')).toBe('#some-heading');
  });
});

describe('vendored docs freshness', () => {
  it('src/content/docs matches a fresh sync of ../docs', () => {
    const fresh = buildDocEntries(sourceDir);
    const vendored = readdirSync(vendoredDir)
      .filter((f) => f.endsWith('.md'))
      .sort();

    expect(vendored).toEqual(fresh.map((e) => `${e.slug}.md`));
    for (const entry of fresh) {
      const onDisk = readFileSync(join(vendoredDir, `${entry.slug}.md`), 'utf8');
      expect(onDisk, `stale vendored copy: run \`pnpm sync-docs\` (${entry.slug}.md)`).toBe(
        entry.contents,
      );
    }
  });
});
