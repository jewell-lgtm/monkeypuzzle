// Cloudflare Turnstile server-side verification for the email-capture action.
//
//   token ──▶ [no secret configured] ──▶ ok (disabled — local dev/tests)
//     │
//     ▼
//   siteverify POST ──▶ success ──▶ ok
//         │                │
//         ▼                ▼
//   network error      success:false
//         │                │
//         ▼                ▼
//     FAIL CLOSED      FAIL CLOSED (visible error, user can retry)
//
// Enforcement is keyed on TURNSTILE_SECRET_KEY being set: production sets it
// (k8s Secret), local dev and unit tests run without it. When enforced, any
// failure — missing token, rejected token, or Cloudflare unreachable — fails
// CLOSED: a bot gate that fails open is not a gate.
const SITEVERIFY_URL = 'https://challenges.cloudflare.com/turnstile/v0/siteverify';

export interface TurnstileResult {
  ok: boolean;
  reason: 'disabled' | 'verified' | 'missing_token' | 'rejected' | 'unreachable';
}

export async function verifyTurnstile(
  token: string | undefined,
  remoteIp: string,
  secret: string | undefined = process.env.TURNSTILE_SECRET_KEY,
  fetchImpl: typeof fetch = fetch,
): Promise<TurnstileResult> {
  if (!secret) return { ok: true, reason: 'disabled' };
  if (!token) return { ok: false, reason: 'missing_token' };
  try {
    const body = new URLSearchParams({ secret, response: token, remoteip: remoteIp });
    const res = await fetchImpl(SITEVERIFY_URL, { method: 'POST', body });
    const data = (await res.json()) as { success?: boolean };
    return data.success === true
      ? { ok: true, reason: 'verified' }
      : { ok: false, reason: 'rejected' };
  } catch {
    return { ok: false, reason: 'unreachable' };
  }
}
