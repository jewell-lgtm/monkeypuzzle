# Releasing

The MIT CLI — `mp` and its MCP bridge `mp-mcp` — is distributed via Homebrew.
Releases are cut by [GoReleaser](https://goreleaser.com) on a version tag. The
FSL-1.1-MIT server (`apps/mp-server`) is **not** shipped through brew.

## Cut a release

```bash
git tag v1.2.3
git push origin v1.2.3
```

`.github/workflows/release.yml` then:

1. builds `mp` + `mp-mcp` for macOS and Linux (amd64 + arm64),
2. creates the GitHub release with archives + `checksums.txt`,
3. writes/updates `Formula/monkeypuzzle.rb` in `jewell-lgtm/homebrew-tap`.

Users install with:

```bash
brew install jewell-lgtm/tap/monkeypuzzle
```

## One-time setup (before the first release)

1. Create a public repo **`jewell-lgtm/homebrew-tap`** (empty is fine — GoReleaser
   commits `Formula/monkeypuzzle.rb` into it).
2. Create a GitHub **Personal Access Token** with `contents:write` scoped to that
   tap repo (fine-grained: repository = `homebrew-tap`). Classic tokens need `repo`.
3. In the **monkeypuzzle** repo, add it as an Actions secret named
   **`HOMEBREW_TAP_GITHUB_TOKEN`** (a separate token is required because the
   built-in `GITHUB_TOKEN` cannot push to another repository).
4. Push a `v*` tag.

## Local checks

```bash
goreleaser check                       # validate .goreleaser.yaml
goreleaser release --snapshot --clean  # full build into ./dist, nothing published
```

## Later: homebrew-core

Once the project clears Homebrew's notability bar (~75+ stars/forks/watchers),
submit `monkeypuzzle` to [homebrew/homebrew-core](https://github.com/Homebrew/homebrew-core)
so the prefix drops to a plain `brew install monkeypuzzle`. The tap keeps working
in the meantime.
