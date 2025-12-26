# Docker Development Environment

This document describes how to use the Docker-based development environment for monkeypuzzle. This is the **preferred way to create reproducible issues** when contributing to the project.

## Overview

The Docker development environment provides:

- **Ubuntu 24.04** base system
- **Go 1.24.11** (matching the project's toolchain)
- **mp CLI** pre-built and available in PATH
- **git** for version control
- **tmux** for terminal multiplexing (required by `mp piece`)
- **gh CLI** (GitHub CLI) for PR management
- **Essential development tools** (vim, nano, curl, wget, etc.)

## Prerequisites

- Docker installed and running
- Basic familiarity with Docker commands

## Quick Start

### 1. Build the Docker Image

From the repository root:

```bash
docker build -t monkeypuzzle-dev .
```

This will:

- Download Ubuntu 24.04 base image
- Install all dependencies
- Build the `mp` command from source
- Set up the development environment

**Build time:** ~2-5 minutes (depending on network speed)

### 2. Run the Container

#### Basic Usage (Read-Only Source)

If you just want to test the pre-built `mp` command:

```bash
docker run -it --rm monkeypuzzle-dev
```

This starts an interactive bash shell with `mp` available.

#### Development Mode (Live Code Changes)

For active development where you want to edit source code and see changes:

```bash
# Mount current directory as workspace
docker run -it --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  monkeypuzzle-dev
```

**Important:** When mounting the source code, the pre-built `mp` binary in the container may be from an older version. To use your local changes, rebuild `mp` inside the container:

```bash
# Inside the container
cd /workspace
go build -o /usr/local/bin/mp .
```

Or use a local binary:

```bash
# Inside the container
cd /workspace
go build -o ./mp .
./mp --help
```

### 3. Verify Installation

Once inside the container, verify everything works:

```bash
# Check mp is available
mp --help

# Check git
git --version

# Check tmux
tmux -V

# Check gh CLI
gh --version

# Check Go version
go version
```

## Common Workflows

### Testing mp Commands

```bash
# Start container with workspace mounted
docker run -it --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  monkeypuzzle-dev

# Inside container: test mp init
mp init --schema

# Test with JSON input
echo '{"name":"test","issue_provider":"markdown","pr_provider":"github"}' | mp init
```

### Developing with Live Reload

1. **Terminal 1:** Start container with mounted source

   ```bash
   docker run -it --rm \
     -v "$(pwd):/workspace" \
     -w /workspace \
     monkeypuzzle-dev
   ```

2. **Terminal 2:** Edit source code on your host machine

3. **Inside container:** Rebuild and test
   ```bash
   go build -o ./mp .
   ./mp --help
   ```

### Using mp piece Command

The `mp piece` command requires git and tmux, both available in the container:

```bash
# Start container
docker run -it --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  monkeypuzzle-dev

# Initialize a project
mp init --name myproject --issue-provider markdown --pr-provider github

# Create a new piece
mp piece new

# This will create a git worktree and tmux session
```

### Running Tests

```bash
docker run -it --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  monkeypuzzle-dev \
  bash -c "go test ./..."
```

### Running Linters

```bash
docker run -it --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  monkeypuzzle-dev \
  bash -c "go vet ./... && go fmt ./..."
```

## Creating Reproducible Issues

This Docker environment is the **preferred way** to create reproducible bug reports and test cases.

### Step 1: Reproduce the Issue

```bash
# Build the image
docker build -t monkeypuzzle-dev .

# Run with your test case
docker run -it --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  monkeypuzzle-dev \
  bash -c "mp <your-command-here>"
```

### Step 2: Document the Environment

Include in your issue:

```markdown
## Environment

- Docker version: `docker --version`
- Image: `monkeypuzzle-dev` (built from Dockerfile)
- OS: Ubuntu 24.04
- Go: 1.24.11
- mp version: `mp --version` (or commit hash)
```

### Step 3: Provide Reproduction Steps

```markdown
## Steps to Reproduce

1. Build image: `docker build -t monkeypuzzle-dev .`
2. Run: `docker run -it --rm -v "$(pwd):/workspace" -w /workspace monkeypuzzle-dev`
3. Execute: `mp <command>`
4. Observe: [describe the issue]
```

### Step 4: Include Minimal Test Case

If possible, create a minimal test case that reproduces the issue:

```bash
# Create a test script
cat > test-reproduce.sh << 'EOF'
#!/bin/bash
set -e
mp init --name test --issue-provider markdown --pr-provider github
# ... more commands that reproduce the issue
EOF

# Run in container
docker run -it --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  monkeypuzzle-dev \
  bash test-reproduce.sh
```

## Advanced Usage

### Custom User ID

If you need to match your host user ID (to avoid permission issues):

```bash
docker run -it --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  -u $(id -u):$(id -g) \
  monkeypuzzle-dev
```

**Note:** The default user in the container is `developer` (UID 1000). If your host user has a different UID, files created in mounted volumes may have different ownership.

### Persistent Go Module Cache

To speed up builds, you can mount the Go module cache:

```bash
docker run -it --rm \
  -v "$(pwd):/workspace" \
  -v go-mod-cache:/go/pkg/mod \
  -w /workspace \
  monkeypuzzle-dev
```

### Running Specific Commands

You can run commands directly without entering the container:

```bash
# Run mp init
docker run -it --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  monkeypuzzle-dev \
  mp init --schema

# Run tests
docker run -it --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  monkeypuzzle-dev \
  go test ./internal/core/init/... -v
```

### Using Docker Compose (Optional)

Create a `docker-compose.yml` for convenience:

```yaml
version: "3.8"

services:
  dev:
    build: .
    image: monkeypuzzle-dev
    volumes:
      - .:/workspace
    working_dir: /workspace
    stdin_open: true
    tty: true
```

Then use:

```bash
docker-compose run --rm dev bash
```

## Troubleshooting

### Permission Issues

If you encounter permission errors when mounting volumes:

```bash
# Option 1: Use your host user ID
docker run -it --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  -u $(id -u):$(id -g) \
  monkeypuzzle-dev

# Option 2: Fix permissions on host
sudo chown -R $USER:$USER .
```

### mp Command Not Found

If `mp` is not available:

```bash
# Rebuild inside container
cd /workspace
go build -o /usr/local/bin/mp .
```

Or use the local binary:

```bash
cd /workspace
go build -o ./mp .
./mp --help
```

### Git Configuration

The container sets safe defaults, but you may want to configure git:

```bash
git config --global user.name "Your Name"
git config --global user.email "your.email@example.com"
```

### GitHub CLI Authentication

If you need to use `gh` CLI (for PR operations):

```bash
# Authenticate (interactive)
gh auth login

# Or use a token
export GH_TOKEN=your_token_here
```

### Network Issues

If you're behind a proxy:

```bash
docker run -it --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  -e HTTP_PROXY=http://proxy:port \
  -e HTTPS_PROXY=http://proxy:port \
  monkeypuzzle-dev
```

## Building for Different Architectures

The Dockerfile is configured for `amd64` by default. For other architectures:

```bash
# Build for ARM64 (Apple Silicon, etc.)
docker build --platform linux/arm64 -t monkeypuzzle-dev .
```

Note: You may need to adjust the `GO_ARCH` variable in the Dockerfile for non-amd64 architectures.

## Best Practices

1. **Always use the Docker environment** when reporting bugs to ensure reproducibility
2. **Mount source code as a volume** for active development
3. **Rebuild mp inside container** when source code changes
4. **Include Docker commands** in bug reports for easy reproduction
5. **Use specific commit hashes** when reporting issues tied to specific code versions

## Integration with CI/CD

The Dockerfile can be used in CI/CD pipelines:

```yaml
# Example GitHub Actions
- name: Test in Docker
  run: |
    docker build -t monkeypuzzle-dev .
    docker run --rm monkeypuzzle-dev go test ./...
```

## See Also

- [Contributing Guide](../CONTRIBUTING.md) - General contribution guidelines
- [Getting Started](./getting-started.md) - Project setup and usage
- [Architecture](./architecture.md) - Project structure and design
