---
title: Plane issue provider
status: in-progress
description: Add a Plane (plane.so) issue provider so users can list, search, and work on issues from a Plane project, mirroring the existing Linear provider. Supports self-hosted Plane via a configurable API base URL.
---

# Plane issue provider

Add a `plane` issue provider implementing `issue.Provider`, so `mp issue list`,
`mp issue search`, and `mp piece create` operate on a Plane project's issues.
Plane can be self-hosted, so the API base URL is configurable (defaults to
`https://api.plane.so`).

Plan file: `~/.claude/plans/i-want-to-be-tranquil-waffle.md`

## Config

`.monkeypuzzle/monkeypuzzle.json`:

```json
{
  "issues": {
    "provider": "plane",
    "config": {
      "workspace": "<workspace-slug>",
      "project": "<project-uuid>",
      "api_key": "<plane-api-token>",
      "base_url": "https://api.plane.so"
    }
  }
}
```

API key may instead come from the `PLANE_API_KEY` env var. `base_url` is optional.
`mp init --issue-provider plane --plane-workspace ... --plane-project ... --plane-api-key ... [--plane-base-url ...]`
writes the same config.

## Status mapping

Plane state `group` → mp status: `backlog`/`unstarted`/`triage` → `todo`;
`started` → `in-progress`; `completed`/`cancelled` → `done`.

## Work

- [x] `internal/core/issue/plane_provider.go` — `PlaneProvider` implementing `issue.Provider`
  (Create / List / SearchIssues / Get / UpdateStatus), cursor pagination, lazy state-group
  and project-identifier caching, `X-API-Key` auth.
- [x] Register `"plane"` in `internal/core/issue/registry.go`.
- [x] `mp init` flags + flags/stdin wiring (`cmd/mp/init.go`), validation
  (`internal/core/init/input.go`), completion + valid values updated.
- [x] Unit tests (`internal/core/issue/plane_provider_test.go`) — happy path, status filter,
  search-by-title, pagination, registry validation.
- [ ] (optional / follow-up) Add Plane fields to the interactive `mp init` TUI
  (`internal/tui/init/`) — currently configurable via flags / stdin JSON only.
- [ ] (optional / follow-up) Live integration test gated on real Plane credentials.
