// Site configuration.
//
// Every link on the site points at a real destination — no dead ends. The
// hosted dashboard is not launched; when it is, its URL comes back here.
// (The old SERVER_URL/LOGIN_URL pair pointed "Get started" at a login page
// that dead-ended at "coming soon" — removed on purpose.)
export const GITHUB_URL = 'https://github.com/jewell-lgtm/monkeypuzzle';
export const DOCS_URL = `${GITHUB_URL}#readme`;

// Cloudflare Turnstile SITE key (public by design — safe to commit; the
// SECRET key lives only in the k8s Secret `monkeypuzzle-website-turnstile`).
// This is Cloudflare's official "always passes, invisible" TEST key.
// TODO(founder): create a Turnstile widget for monkeypuzzle.dev at
// https://dash.cloudflare.com → Turnstile, then replace this with the real
// site key and set the secret on the cluster (see gitops deployment.yaml).
export const TURNSTILE_SITE_KEY = '1x00000000000000000000AA';
