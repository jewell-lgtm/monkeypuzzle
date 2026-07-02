import { defineAction } from 'astro:actions';
import { z } from 'astro:schema';
import { subscribe, MAX_EMAIL_LENGTH } from '../lib/waitlist';

export const server = {
  // Email capture for the CTA band. `accept: 'form'` rejects non-form content
  // types; real validation/normalization/dedupe/rate-limiting lives in
  // src/lib/waitlist.ts (unit-tested there). The handler always RETURNS a
  // result — form outcomes are states, not exceptions.
  subscribe: defineAction({
    accept: 'form',
    input: z.object({
      email: z.string().max(MAX_EMAIL_LENGTH),
    }),
    handler: async ({ email }, ctx) => {
      let ip = 'unknown';
      try {
        ip = ctx.clientAddress;
      } catch {
        // clientAddress throws on prerendered contexts; actions are always
        // server-rendered, but stay defensive — rate limiting degrades to a
        // shared bucket rather than crashing the submission.
      }
      return subscribe(email, ip);
    },
  }),
};
