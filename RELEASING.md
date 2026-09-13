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

The public [`jewell-lgtm/homebrew-tap`](https://github.com/jewell-lgtm/homebrew-tap)
repository already exists. The remaining setup is deliberately manual so a
broad developer OAuth token is never copied into Actions:

1. Create a GitHub **fine-grained Personal Access Token** with `contents:write` scoped to that
   tap repo (fine-grained: repository = `homebrew-tap`). Classic tokens need `repo`.
2. In the **monkeypuzzle** repo, add it as an Actions secret named
   **`HOMEBREW_TAP_GITHUB_TOKEN`** (a separate token is required because the
   built-in `GITHUB_TOKEN` cannot push to another repository).
3. Confirm it without exposing the value:

   ```bash
   gh secret list --repo jewell-lgtm/monkeypuzzle | grep HOMEBREW_TAP_GITHUB_TOKEN
   ```

4. Push a `v*` tag. The release job runs the full Go test suite before publishing;
   after publication, clean macOS and Linux runners install the public formula and
   exercise `mp`, `mp-mcp`, piece creation, and an in-worktree branch stack.

## Local checks

```bash
goreleaser check                       # validate .goreleaser.yaml
goreleaser release --snapshot --clean  # full build into ./dist, nothing published
```

Every pull request that changes the binaries or release configuration runs the
same snapshot build in `.github/workflows/release-check.yml`.

GoReleaser is pinned to v2.9.0. GoReleaser v2.10 deprecated formula publishing
in favor of casks, but casks are macOS-only; Monkeypuzzle deliberately retains a
formula so the same tap supports macOS and Linuxbrew. Revisit the pin when there
is a supported cross-platform successor, not as an incidental dependency bump.

## Later: homebrew-core

Once the project clears Homebrew's notability bar (~75+ stars/forks/watchers),
submit `monkeypuzzle` to [homebrew/homebrew-core](https://github.com/Homebrew/homebrew-core)
so the prefix drops to a plain `brew install monkeypuzzle`. The tap keeps working
in the meantime.
