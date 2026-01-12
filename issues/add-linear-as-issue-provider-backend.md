---
title: Add Linear as issue provider backend
status: done
description: "Extend issue provider interface to be fully pluggable, then add Linear as first external backend.

## Background

The `Provider` interface exists (`internal/core/issue/provider.go`) but has gaps preventing true pluggability:
- Hardcoded provider instantiation (no factory/registry)
- FS-coupled dependencies (MarkdownProvider uses `core.FS`)
- Fixed status enum assumes todo/in-progress/done (Linear has custom states)
- No capabilities discovery

## Phase 1: Make interface fully pluggable

- [x] Add provider factory/registry pattern in `issue/registry.go`
- [x] Add `ProviderFactory` interface or `RegisterProvider()` function
- [x] Support per-provider dependency injection (HTTP client, auth tokens vs FS)
- [x] Add status mapping layer (provider status <-> mp status)
- [x] Update init validation to use provider registry instead of hardcoded list
- [ ] Add optional capabilities interface (pagination, labels, projects, etc.) - deferred
- [x] Update `Handler.getProvider()` to use factory

## Phase 2: Add Linear provider

- [x] Create `internal/core/issue/linear_provider.go`
- [x] Implement Provider interface for Linear API
- [x] Map Linear workflow states to mp statuses (Backlog/Todo -> todo, In Progress -> in-progress, Done/Canceled -> done)
- [x] Store Linear API key in config or env var
- [x] Handle Linear team/project selection in config
- [x] Add integration test with mocked Linear API
- [ ] Update docs/README with Linear setup instructions - deferred

## Config example

```json
{
  \"issues\": {
    \"provider\": \"linear\",
    \"config\": {
      \"team\": \"ENG\",
      \"project\": \"monkeypuzzle\"
    }
  }
}
```

## Acceptance criteria

- Can switch between markdown and linear providers via config
- `mp issue create/list/get` work with Linear backend
- Status transitions sync correctly between mp and Linear
- No breaking changes to existing markdown provider usage"
---

# Add Linear as issue provider backend

Extend issue provider interface to be fully pluggable, then add Linear as first external backend.

## Background

The `Provider` interface exists (`internal/core/issue/provider.go`) but has gaps preventing true pluggability:
- Hardcoded provider instantiation (no factory/registry)
- FS-coupled dependencies (MarkdownProvider uses `core.FS`)
- Fixed status enum assumes todo/in-progress/done (Linear has custom states)
- No capabilities discovery

## Phase 1: Make interface fully pluggable

- [x] Add provider factory/registry pattern in `issue/registry.go`
- [x] Add `ProviderFactory` interface or `RegisterProvider()` function
- [x] Support per-provider dependency injection (HTTP client, auth tokens vs FS)
- [x] Add status mapping layer (provider status <-> mp status)
- [x] Update init validation to use provider registry instead of hardcoded list
- [ ] Add optional capabilities interface (pagination, labels, projects, etc.) - deferred
- [x] Update `Handler.getProvider()` to use factory

## Phase 2: Add Linear provider

- [x] Create `internal/core/issue/linear_provider.go`
- [x] Implement Provider interface for Linear API
- [x] Map Linear workflow states to mp statuses (Backlog/Todo -> todo, In Progress -> in-progress, Done/Canceled -> done)
- [x] Store Linear API key in config or env var
- [x] Handle Linear team/project selection in config
- [x] Add integration test with mocked Linear API
- [ ] Update docs/README with Linear setup instructions - deferred

## Config example

```json
{
  "issues": {
    "provider": "linear",
    "config": {
      "team": "ENG",
      "project": "monkeypuzzle"
    }
  }
}
```

## Acceptance criteria

- Can switch between markdown and linear providers via config
- `mp issue create/list/get` work with Linear backend
- Status transitions sync correctly between mp and Linear
- No breaking changes to existing markdown provider usage
