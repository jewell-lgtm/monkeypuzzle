# Dockerfile for monkeypuzzle development environment
#
# This Dockerfile creates a reproducible Ubuntu-based development environment
# with the mp CLI tool, git, tmux, and other essential tools pre-installed.
#
# Usage:
#   docker build -t monkeypuzzle-dev .
#   docker run -it -v $(pwd):/workspace monkeypuzzle-dev
#
# For development with live code changes:
#   docker run -it -v $(pwd):/workspace -w /workspace monkeypuzzle-dev

FROM ubuntu:24.04

# Prevent interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# Install system dependencies and essential tools
RUN apt-get update && apt-get install -y \
    # Build essentials
    build-essential \
    ca-certificates \
    curl \
    wget \
    # Version control
    git \
    # Terminal multiplexer (required by mp piece command)
    tmux \
    # Text editors
    vim \
    nano \
    # Utilities
    less \
    sudo \
    # Cleanup
    && rm -rf /var/lib/apt/lists/*

# Install Go 1.24.11 (matching go.mod toolchain)
# Using the official Go binary distribution
ENV GO_VERSION=1.24.11
ENV GO_ARCH=amd64
RUN wget -q https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz && \
    tar -C /usr/local -xzf go${GO_VERSION}.linux-${GO_ARCH}.tar.gz && \
    rm go${GO_VERSION}.linux-${GO_ARCH}.tar.gz

# Set up Go environment
ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="/go"
ENV PATH="${GOPATH}/bin:${PATH}"

# Install GitHub CLI (gh) - required for PR provider
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | \
    dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg && \
    chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg && \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | \
    tee /etc/apt/sources.list.d/github-cli.list > /dev/null && \
    apt-get update && \
    apt-get install -y gh && \
    rm -rf /var/lib/apt/lists/*

# Create a non-root user for development (mp may need to create files/dirs)
ARG USERNAME=developer
ARG USER_UID=1000
ARG USER_GID=1000

RUN (groupadd --gid ${USER_GID} ${USERNAME} 2>/dev/null || true) && \
    useradd --uid ${USER_UID} --gid ${USER_GID} --shell /bin/bash --create-home ${USERNAME} 2>/dev/null || \
    usermod -l ${USERNAME} -d /home/${USERNAME} -m $(id -nu ${USER_UID}) && \
    # Allow user to use sudo without password (useful for some operations)
    echo "${USERNAME} ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers && \
    # Create go workspace directory
    mkdir -p ${GOPATH} && \
    chown -R ${USER_UID}:${USER_GID} ${GOPATH}

# Set working directory
WORKDIR /workspace

# Copy source code and build mp command
# Note: This builds mp from the source at build time
# For live development, mount the source as a volume
COPY --chown=${USERNAME}:${USERNAME} . /workspace

# Build the mp binary
RUN cd /workspace && \
    go mod download && \
    go build -o /usr/local/bin/mp . && \
    chmod +x /usr/local/bin/mp && \
    # Verify installation
    mp --help > /dev/null || true

# Switch to non-root user
USER ${USERNAME}

# Set up git config defaults (user can override)
RUN git config --global init.defaultBranch main && \
    git config --global --add safe.directory '*'

# Create a convenient entrypoint script
# This allows the container to start directly into a shell
COPY --chown=${USERNAME}:${USERNAME} docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# Default to bash shell
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["bash"]
