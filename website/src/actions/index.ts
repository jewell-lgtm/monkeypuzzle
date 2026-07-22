import { defineAction } from 'astro:actions';
import { z } from 'astro:schema';
import { subscribe, MAX_EMAIL_LENGTH } from '../lib/waitlist';
import { verifyTurnstile } from '../lib/turnstile';

export const server = {
  // Email capture for the CTA band. `accept: 'form'` rejects non-form content
  // types; real validation/normalization/dedupe/rate-limiting lives in
  // src/lib/waitlist.ts (unit-tested there). The handler always RETURNS a
  // result — form outcomes are states, not exceptions.
  //
  // Bot gate, two layers, checked before anything touches the sink:
  //  1. Honeypot — a hidden "website" field humans never see. Filled = bot;
  //     respond with a fake success and write NOTHING (don't tip the bot off).
  //  2. Cloudflare Turnstile — token verified server-side; fails CLOSED when
  //     TURNSTILE_SECRET_KEY is configured (see src/lib/turnstile.ts).
  subscribe: defineAction({
    accept: 'form',
    input: z.object({
      email: z.string().max(MAX_EMAIL_LENGTH),
      website: z.string().optional(), // honeypot — must stay empty
      'cf-turnstile-response': z.string().optional(),
    }),
    handler: async (input, ctx) => {
      let ip = 'unknown';
      try {
        ip = ctx.clientAddress;
      } catch {
        // clientAddress throws on prerendered contexts; actions are always
        // server-rendered, but stay defensive — rate limiting degrades to a
        // shared bucket rather than crashing the submission.
      }
      if (input.website) {
        // Honeypot tripped: only bots fill invisible fields. Fake the happy
        // path so the bot moves on; nothing is written.
        return { ok: true, code: 'added' as const, message: "You're in — we'll email you when things ship." };
      }
      const gate = await verifyTurnstile(input['cf-turnstile-response'], ip);
      if (!gate.ok) {
        return {
          ok: false,
          code: 'bot_check' as const,
          message: "Couldn't confirm you're human — give it a second and try again.",
        };
      }
      return subscribe(input.email, ip);
    },
  }),
};
