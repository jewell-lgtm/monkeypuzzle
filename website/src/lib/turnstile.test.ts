// Unit tests for the Turnstile gate — one per branch of the verify flow:
// disabled / verified / missing token / rejected / unreachable (fail closed).
import { describe, expect, it, vi } from 'vitest';
import { verifyTurnstile } from './turnstile';

const okFetch = (success: boolean) =>
  vi.fn(async () => new Response(JSON.stringify({ success }), { status: 200 })) as unknown as typeof fetch;

describe('verifyTurnstile', () => {
  it('passes with reason=disabled when no secret is configured (local dev/tests)', async () => {
    const res = await verifyTurnstile('any-token', '1.2.3.4', undefined);
    expect(res).toEqual({ ok: true, reason: 'disabled' });
  });

  it('fails CLOSED on a missing token when enforcement is on', async () => {
    const res = await verifyTurnstile(undefined, '1.2.3.4', 'secret');
    expect(res).toEqual({ ok: false, reason: 'missing_token' });
  });

  it('verifies a good token via siteverify', async () => {
    const fetchImpl = okFetch(true);
    const res = await verifyTurnstile('good', '1.2.3.4', 'secret', fetchImpl);
    expect(res).toEqual({ ok: true, reason: 'verified' });
    const [url, init] = (fetchImpl as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(String(url)).toContain('challenges.cloudflare.com/turnstile/v0/siteverify');
    expect(String(init.body)).toContain('response=good');
  });

  it('rejects when siteverify says success=false', async () => {
    const res = await verifyTurnstile('bad', '1.2.3.4', 'secret', okFetch(false));
    expect(res).toEqual({ ok: false, reason: 'rejected' });
  });

  it('fails CLOSED when Cloudflare is unreachable', async () => {
    const boom = vi.fn(async () => {
      throw new Error('network down');
    }) as unknown as typeof fetch;
    const res = await verifyTurnstile('good', '1.2.3.4', 'secret', boom);
    expect(res).toEqual({ ok: false, reason: 'unreachable' });
  });
});
