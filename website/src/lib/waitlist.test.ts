// Unit tests for the waitlist sink — one test per handler branch:
// added / exists (normalized) / invalid / rate_limited / closed (cap) / error (disk).
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { MAX_LINES, RATE_LIMIT_PER_WINDOW, resetRateLimiter, subscribe } from './waitlist';

let dir: string;
let file: string;
let ipCounter = 0;
// Unique IP per test so the shared in-process rate limiter can't leak between tests.
const nextIp = () => `10.0.0.${++ipCounter}`;

beforeEach(async () => {
  dir = await mkdtemp(join(tmpdir(), 'waitlist-'));
  file = join(dir, 'nested', 'waitlist.jsonl');
  resetRateLimiter();
});

afterEach(async () => {
  await rm(dir, { recursive: true, force: true });
});

describe('subscribe', () => {
  it('appends a valid email (creating the directory) and reports added', async () => {
    const res = await subscribe('Dev@Example.com', nextIp(), file);
    expect(res).toMatchObject({ ok: true, code: 'added' });
    const lines = (await readFile(file, 'utf8')).split('\n').filter(Boolean);
    expect(lines).toHaveLength(1);
    const entry = JSON.parse(lines[0]);
    expect(entry.email).toBe('dev@example.com'); // normalized before persisting
    expect(entry.ts).toBeTruthy();
  });

  it('dedupes on the NORMALIZED email — case and whitespace variants are one signup', async () => {
    const ip = nextIp();
    await subscribe('dev@example.com', ip, file);
    const res = await subscribe('  DEV@EXAMPLE.COM  ', ip, file);
    expect(res).toMatchObject({ ok: true, code: 'exists' });
    const lines = (await readFile(file, 'utf8')).split('\n').filter(Boolean);
    expect(lines).toHaveLength(1);
  });

  it('rejects invalid input with a visible error and writes nothing', async () => {
    for (const bad of ['', '   ', 'not-an-email', 'a@b', `${'x'.repeat(250)}@example.com`]) {
      const res = await subscribe(bad, nextIp(), file);
      expect(res).toMatchObject({ ok: false, code: 'invalid' });
    }
    await expect(readFile(file, 'utf8')).rejects.toThrow(); // never created
  });

  it('rate-limits a single IP past the window allowance', async () => {
    const ip = nextIp();
    for (let i = 0; i < RATE_LIMIT_PER_WINDOW; i++) {
      const res = await subscribe(`ok${i}@example.com`, ip, file);
      expect(res.code).toBe('added');
    }
    const res = await subscribe('straw@example.com', ip, file);
    expect(res).toMatchObject({ ok: false, code: 'rate_limited' });
  });

  it('refuses new signups past the file cap, visibly', async () => {
    const lines = Array.from({ length: MAX_LINES }, (_, i) =>
      JSON.stringify({ email: `u${i}@example.com`, ts: 't' }),
    );
    const flat = join(dir, 'full.jsonl');
    await writeFile(flat, lines.join('\n') + '\n', 'utf8');
    const res = await subscribe('late@example.com', nextIp(), flat);
    expect(res).toMatchObject({ ok: false, code: 'closed' });
  });

  it('surfaces disk failures as a visible error, never silence', async () => {
    // A path THROUGH a regular file cannot be created — append must fail.
    const blocker = join(dir, 'blocker');
    await writeFile(blocker, 'i am a file', 'utf8');
    const res = await subscribe('dev@example.com', nextIp(), join(blocker, 'child.jsonl'));
    expect(res).toMatchObject({ ok: false, code: 'error' });
    expect(res.message.length).toBeGreaterThan(0);
  });

  it('serializes concurrent submissions of the same email into one line', async () => {
    const results = await Promise.all(
      Array.from({ length: 4 }, () => subscribe('race@example.com', nextIp(), file)),
    );
    expect(results.filter((r) => r.code === 'added')).toHaveLength(1);
    expect(results.filter((r) => r.code === 'exists')).toHaveLength(3);
    const lines = (await readFile(file, 'utf8')).split('\n').filter(Boolean);
    expect(lines).toHaveLength(1);
  });
});
