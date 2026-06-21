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
# Bake the server URL the static site links to (login/dashboard). Generic by
# default (config.ts falls back to localhost); deployments pass their real URL,
# e.g. --build-arg PUBLIC_SERVER_URL=https://app.example.com
ARG PUBLIC_SERVER_URL=""
ENV PUBLIC_SERVER_URL=${PUBLIC_SERVER_URL}
RUN pnpm build

FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
