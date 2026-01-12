---
title: Fix IsBranchMerged returning success with empty Method
status: done
priority: critical
---

# Fix IsBranchMerged returning success with empty Method

## Problem

`IsBranchMerged()` in `handler.go:837` returns `(MergeStatus, nil)` when all four merge detection methods fail, without setting `status.Method`. This returns "success" with incomplete data.

## Impact

- Callers cannot distinguish "confirmed not merged" from "unable to determine"
- `CleanupMergedPieces` may skip cleanup of actually-merged pieces
- Unreliable merge detection

## Location

- `internal/core/piece/handler.go:837`

## Fix

Either return an error when all methods fail, or set Method to "unknown":

```go
// At end of IsBranchMerged, before final return:
status.Method = "inconclusive"
return status, nil
// OR
return status, fmt.Errorf("could not determine merge status")
```

## Testing

- Add test case where all four merge detection methods fail
- Verify return value indicates inconclusive result
