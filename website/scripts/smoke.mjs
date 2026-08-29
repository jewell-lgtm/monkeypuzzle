// Route smoke test (regression guard for the nginx→node swap): every
// prerendered route must serve 200 with expected content from the node
// standalone server. Run after `pnpm build`: `pnpm smoke`.
import { spawn } from 'node:child_process';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const PORT = process.env.SMOKE_PORT ?? '4399';
const ROUTES = [
  { path: '/', mustContain: 'Land your agents' },
  { path: '/workflow', mustContain: 'mp' },
  { path: '/docs/', mustContain: 'Guides' },
  { path: '/docs/getting-started/', mustContain: 'mp' },
  { path: '/docs/commands/', mustContain: 'multiplexer' },
];

const server = spawn('node', ['dist/server/entry.mjs'], {
  env: {
    ...process.env,
    HOST: '127.0.0.1',
    PORT,
    // Point the sink at a throwaway dir so a smoke POST can never touch real data.
    WAITLIST_FILE: join(mkdtempSync(join(tmpdir(), 'smoke-')), 'waitlist.jsonl'),
  },
  stdio: ['ignore', 'pipe', 'pipe'],
});
server.stderr.on('data', (d) => process.stderr.write(d));

const fail = async (msg) => {
  console.error(`SMOKE FAIL: ${msg}`);
  server.kill();
  process.exit(1);
};

// Wait for the server to accept connections.
let up = false;
for (let i = 0; i < 50 && !up; i++) {
  try {
    await fetch(`http://127.0.0.1:${PORT}/`);
    up = true;
  } catch {
    await new Promise((r) => setTimeout(r, 200));
  }
}
if (!up) await fail('server did not start within 10s');

for (const { path, mustContain } of ROUTES) {
  const res = await fetch(`http://127.0.0.1:${PORT}${path}`);
  if (res.status !== 200) await fail(`${path} returned ${res.status}`);
  const body = await res.text();
  if (!body.includes(mustContain)) await fail(`${path} missing expected content "${mustContain}"`);
  console.log(`ok ${path} (200, content matched)`);
}

server.kill();
console.log('SMOKE PASS');
process.exit(0);
