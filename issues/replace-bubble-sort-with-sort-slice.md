---
title: Replace bubble sort with sort.Slice
status: in-progress
description: "handler.go:883-889 uses O(n²) bubble sort. Replace with sort.Slice for O(n log n)."
---

# Replace bubble sort with sort.Slice

handler.go:883-889 uses O(n²) bubble sort. Replace with sort.Slice for O(n log n).
