---
title: newPieceHandler recurses infinitely on bad user config
---

# newPieceHandler recurses infinitely on bad user config

Both error paths in cmd/mp/piece.go newPieceHandler called themselves instead
of falling back to the noop multiplexer — corrupt config.json or an unknown
multiplexer value crashed every piece command with a stack overflow.

Fixed: degrade to NoopMultiplexer with a stderr warning. Covered by
TestNewPieceHandler_FallsBackOnBadConfig (red before fix, green after).
