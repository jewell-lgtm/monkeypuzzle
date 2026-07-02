// Waitlist sink: append-only JSONL on a mounted volume.
//
//   submit ──▶ validate ──▶ rate-limit ──▶ [queue] read ──▶ cap? ──▶ dedupe ──▶ append
//                │invalid       │too many            │ENOENT=empty  │closed   │exists    │added
//                ▼              ▼                     ▼              ▼         ▼          ▼
//              visible        visible              visible error   visible   ok         ok
//
// Design constraints (from the eng review of the evidence-sprint plan):
// - Emails are normalized (lowercase+trim) BEFORE dedupe.
// - Writes are serialized through an in-process queue: the read→dedupe→append
//   sequence is not atomic on its own. Single-replica semantics by design.
// - The file is capped; hitting the cap is a VISIBLE error, never silence.
// - Every failure path returns a user-readable message (zero silent failures).
import { appendFile, mkdir, readFile } from 'node:fs/promises';
import { dirname } from 'node:path';

export type SubscribeCode =
  | 'added'
  | 'exists'
  | 'invalid'
  | 'rate_limited'
  | 'closed'
  | 'error';

export interface SubscribeResult {
  ok: boolean;
  code: SubscribeCode;
  message: string;
}

export const MAX_EMAIL_LENGTH = 254;
export const MAX_LINES = 10_000;
export const RATE_LIMIT_PER_WINDOW = 5;
export const RATE_WINDOW_MS = 60_000;

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export function normalizeEmail(raw: string): string {
  return raw.trim().toLowerCase();
}

export function isValidEmail(email: string): boolean {
  return email.length > 0 && email.length <= MAX_EMAIL_LENGTH && EMAIL_RE.test(email);
}

// In-process, per-IP sliding window. Resets on restart; fine at 1 replica.
const hits = new Map<string, number[]>();

export function isRateLimited(ip: string, now: number = Date.now()): boolean {
  const recent = (hits.get(ip) ?? []).filter((t) => now - t < RATE_WINDOW_MS);
  recent.push(now);
  hits.set(ip, recent);
  return recent.length > RATE_LIMIT_PER_WINDOW;
}

export function resetRateLimiter(): void {
  hits.clear();
}

function defaultFile(): string {
  return process.env.WAITLIST_FILE ?? './data/waitlist.jsonl';
}

// Serialize read→dedupe→append; the queue never rejects (failures are results).
let queue: Promise<unknown> = Promise.resolve();

export function subscribe(
  rawEmail: string,
  ip: string,
  file: string = defaultFile(),
): Promise<SubscribeResult> {
  const run = queue.then(() => doSubscribe(rawEmail, ip, file));
  queue = run.catch(() => {});
  return run;
}

async function doSubscribe(rawEmail: string, ip: string, file: string): Promise<SubscribeResult> {
  const email = normalizeEmail(rawEmail);
  if (!isValidEmail(email)) {
    return { ok: false, code: 'invalid', message: "That doesn't look like an email address." };
  }
  if (isRateLimited(ip)) {
    return { ok: false, code: 'rate_limited', message: 'Too many attempts — try again in a minute.' };
  }
  try {
    let lines: string[] = [];
    try {
      lines = (await readFile(file, 'utf8')).split('\n').filter(Boolean);
    } catch (err) {
      if ((err as NodeJS.ErrnoException).code !== 'ENOENT') throw err;
    }
    if (lines.length >= MAX_LINES) {
      return { ok: false, code: 'closed', message: 'Signups are paused right now — try again later.' };
    }
    const seen = lines.some((line) => {
      try {
        return (JSON.parse(line) as { email?: string }).email === email;
      } catch {
        return false;
      }
    });
    if (seen) {
      return { ok: true, code: 'exists', message: "You're already on the list — nothing more to do." };
    }
    await mkdir(dirname(file), { recursive: true });
    await appendFile(file, JSON.stringify({ email, ts: new Date().toISOString() }) + '\n', 'utf8');
    return { ok: true, code: 'added', message: "You're in — we'll email you when things ship." };
  } catch {
    return { ok: false, code: 'error', message: 'Something broke on our side — please try again shortly.' };
  }
}
