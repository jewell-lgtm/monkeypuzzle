// Site configuration.
//
// SERVER_URL points at the mp-server web app (login + dashboard). It defaults
// to the server's local dev address (PORT 8080); override at build time with
// PUBLIC_SERVER_URL, e.g. `PUBLIC_SERVER_URL=https://app.monkeypuzzle.dev`.
export const SERVER_URL: string =
  import.meta.env.PUBLIC_SERVER_URL ?? 'http://localhost:8080';

// Logging in starts the server's WorkOS OAuth flow (GET /login → redirect).
export const LOGIN_URL = `${SERVER_URL}/login`;

export const GITHUB_URL = 'https://github.com/jewell-lgtm/monkeypuzzle';
export const DOCS_URL = `${GITHUB_URL}#readme`;
