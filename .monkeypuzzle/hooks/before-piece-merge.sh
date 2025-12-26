#!/bin/bash
set -e

echo "Running tests before merge..."
cd "$MP_WORKTREE_PATH"

go test ./... || {
    echo "Tests failed - merge aborted"
    exit 1
}

go vet ./... || {
    echo "Vet failed - merge aborted"
    exit 1
}

echo "All checks passed"
