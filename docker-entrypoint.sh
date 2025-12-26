#!/bin/bash
# Docker entrypoint script for monkeypuzzle development environment
#
# This script sets up the environment and executes the provided command.
# If no command is provided, it starts an interactive bash shell.

set -e

# If the first argument is a command, execute it
# Otherwise, start bash
if [ $# -eq 0 ]; then
    exec bash
else
    exec "$@"
fi
