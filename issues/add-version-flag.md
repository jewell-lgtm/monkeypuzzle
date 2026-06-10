---
title: Add mp --version wired via ldflags
---

# Add mp --version wired via ldflags

`mp --version` doesn't exist. Beta users can't verify their install or report
which version a bug is against.

- Wire cobra's `Version` field on rootCmd
- Inject version via `-ldflags "-X ...Version=..."` in Makefile (default `dev`)
- `go install` builds report `dev` (or module version via debug.ReadBuildInfo fallback)
- Integration test: `mp --version` prints something non-empty
