# Self-hosting the Monkeypuzzle server

The `mp` CLI is free and runs entirely in your terminal. The **server** adds a
web dashboard + MCP endpoint that syncs your GitHub repos and GitLab projects into
Postgres and draws each stacked PR/MR tree as a live forest. It's source-available under FSL-1.1-MIT —
self-hosting and internal use are free.

This guide installs the server on Kubernetes with the official Helm chart in
[`deploy/charts/monkeypuzzle`](../deploy/charts/monkeypuzzle). A single
`helm install` brings up the whole stack: the web/MCP server, the sync worker,
and bundled Postgres + Temporal. Point at external Postgres/Temporal for
production.

> Just kicking the tyres? `cd apps/mp-server && docker compose up --build` runs
> the same stack with Docker Compose. See
> [`apps/mp-server`](../apps/mp-server). The Helm chart is the supported path for
> a durable, internet-reachable deployment.

## Architecture

| Component | What it is | Chart resource |
| --- | --- | --- |
| **server** (`mp-server serve`) | HTTP: HTML dashboard for humans + MCP endpoint for agents | Deployment + Service (+ Ingress) |
| **worker** (`mp-server worker`) | Temporal worker that syncs GitHub/GitLab → Postgres | Deployment |
| **Postgres** | App cache + encrypted token store | StatefulSet (bundled) or external |
| **Temporal** | Drives the sync workflows | Deployment (bundled dev server) or external |

The server and worker run the **same image** with different commands.

## Prerequisites

- A Kubernetes cluster and `kubectl` context pointing at it.
- [Helm](https://helm.sh) 3.8+ (or 4.x).
- An ingress controller (e.g. Traefik, nginx) and a DNS name pointing at it, if
  you want the server reachable over the internet. **The public URL is required
  for OAuth to work** — WorkOS redirects back to it and MCP clients use it as the
  resource indicator.
- A [WorkOS](https://dashboard.workos.com) account (free tier is fine).

## 1. Configure WorkOS

The server delegates login to WorkOS AuthKit, which brokers GitHub OAuth for both
humans (web) and agents (MCP). In the WorkOS dashboard:

1. **Create an AuthKit application** and add a **GitHub OAuth connection**. Turn
   **"Return GitHub OAuth tokens" ON** — the server needs the user's GitHub token
   to read their repos and PRs.
2. Set the **redirect URI** to `https://<your-host>/auth/callback`.
3. Add a **Resource Indicator** equal to your public base URL
   (`https://<your-host>`), and enable **Dynamic Client Registration** /
   **Client ID Metadata Document** so MCP clients can register.
4. Note your **API key** (`sk_...`), **Client ID** (`client_...`), and **AuthKit
   domain** (`https://<your-app>.authkit.app`).

### (Optional) GitLab login

WorkOS has no built-in GitLab connector, so GitLab sign-in uses a **direct
GitLab OAuth2** leg instead — GitHub keeps using WorkOS unchanged. It is opt-in:
the GitLab login button only appears when both `GITLAB_OAUTH_CLIENT_ID` and
`GITLAB_OAUTH_CLIENT_SECRET` are set.

1. In GitLab (**User settings → Applications**, or a group/instance application),
   create an OAuth application with scopes **`read_api`** and **`read_user`** and
   redirect URI `https://<your-host>/auth/callback`.
2. Provide the credentials via env:

   ```bash
   GITLAB_OAUTH_CLIENT_ID=...        # GitLab application id
   GITLAB_OAUTH_CLIENT_SECRET=...    # GitLab application secret
   GITLAB_BASE_URL=https://gitlab.com  # or your self-managed instance URL
   ```

## 2. Generate server secrets

```bash
# Cookie signing key (>= 16 bytes) and the AES-256-GCM token key (exactly 32 bytes).
openssl rand -base64 32   # -> secrets.sessionSecret
openssl rand -base64 32   # -> secrets.tokenEncryptionKey
```

## 3. Write your values

Create `my-values.yaml` (keep it out of version control — it holds secrets):

```yaml
# Public, internet-reachable URL. Must match the WorkOS redirect/resource.
publicBaseURL: https://mp.example.com
secureCookies: true

image:
  # The server image. Build + push your own (see §6) or use a published tag.
  repository: ghcr.io/jewell-lgtm/monkeypuzzle/mp-server
  tag: "0.1.0"

workos:
  apiKey: sk_live_xxx
  clientId: client_xxx
  authkitDomain: https://your-app.authkit.app

secrets:
  sessionSecret: <openssl rand -base64 32>
  tokenEncryptionKey: <openssl rand -base64 32>

ingress:
  enabled: true
  className: traefik          # or nginx, etc.
  host: mp.example.com
  # tls:
  #   - secretName: mp-tls
  #     hosts: [mp.example.com]
```

Prefer to manage secrets out of band? Create a Kubernetes Secret yourself with the
keys `DATABASE_URL`, `WORKOS_API_KEY`, `WORKOS_CLIENT_ID`, `AUTHKIT_DOMAIN`,
`SESSION_SECRET`, `TOKEN_ENCRYPTION_KEY` and set `existingSecret: <name>` — the
chart will reference it instead of templating one from values.

## 4. Install

```bash
helm install mp deploy/charts/monkeypuzzle \
  --namespace monkeypuzzle --create-namespace \
  -f my-values.yaml

kubectl -n monkeypuzzle rollout status deploy/mp-monkeypuzzle-server
```

The server runs its own database migrations on startup, so there's no separate
migration step. Once the rollout is healthy, open `https://mp.example.com` and
sign in with GitHub.

No ingress? Port-forward instead (OAuth won't complete without a public URL, but
the UI loads):

```bash
kubectl -n monkeypuzzle port-forward svc/mp-monkeypuzzle-server 8080:80
```

## 5. Connect an MCP client

The server is also an MCP endpoint at `https://mp.example.com/mcp`. Point your
agent at it; it discovers auth via
`https://mp.example.com/.well-known/oauth-protected-resource` and registers
dynamically through WorkOS.

## 6. Building the server image

Until a public image is published for your platform you can build and push your
own — the server and worker share one image:

```bash
docker build -f apps/mp-server/Dockerfile -t <registry>/mp-server:<tag> .
docker push <registry>/mp-server:<tag>
# then set image.repository / image.tag in your values
```

## Production notes

The bundled Postgres and Temporal make `helm install` a one-liner, but for a
durable production deployment use managed/external instances:

```yaml
postgres:
  enabled: false
externalDatabase:
  url: postgres://user:pass@db.internal:5432/mp?sslmode=require

temporal:
  enabled: false
externalTemporal:
  hostPort: temporal-frontend.temporal.svc.cluster.local:7233
```

The bundled Temporal is a single-replica `start-dev` server (in-memory — its
state is lost on restart). Sync workflows are idempotent re-syncs, so a restart
only triggers a re-sync — but it is **not** a production Temporal. App data (repos, PRs, encrypted tokens) lives in
Postgres, so persist *that* (the bundled Postgres uses a PVC by default).

## Upgrading

```bash
helm upgrade mp deploy/charts/monkeypuzzle -n monkeypuzzle -f my-values.yaml
```

Pods roll automatically when the rendered secret changes (a checksum annotation
forces it). New migrations apply on server startup.

## Uninstalling

```bash
helm uninstall mp -n monkeypuzzle
# The Postgres PVC is retained by default; delete it to drop the data:
kubectl -n monkeypuzzle delete pvc -l app.kubernetes.io/instance=mp
```

## Configuration reference

See [`deploy/charts/monkeypuzzle/values.yaml`](../deploy/charts/monkeypuzzle/values.yaml)
for every value and its default.
