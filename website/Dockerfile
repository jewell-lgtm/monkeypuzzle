# Build context is the website/ directory: `docker build website/`.
# syntax=docker/dockerfile:1
FROM node:22-alpine AS build
WORKDIR /app
# Pin pnpm: corepack defaults to latest (pnpm 11), which makes the
# ignored-build-scripts notice fatal (ERR_PNPM_IGNORED_BUILDS) on install.
RUN corepack enable && corepack prepare pnpm@9.15.9 --activate
COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY . .
RUN pnpm build

# Node runtime (replaces nginx): pages are prerendered, but the email-capture
# Astro Action needs a server. Standalone build bundles its dependencies —
# only dist/ is needed at runtime.
FROM node:22-alpine
WORKDIR /app
ENV HOST=0.0.0.0 PORT=4321 NODE_ENV=production
# The waitlist JSONL must live on a mounted volume (k8s: PVC at /data) —
# container-local storage would silently lose signups on every restart.
ENV WAITLIST_FILE=/data/waitlist.jsonl
COPY --from=build /app/dist ./dist
EXPOSE 4321
CMD ["node", "dist/server/entry.mjs"]
