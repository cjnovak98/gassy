# Build stage for supervisor
# Build context should be /workspace/group/gassy (parent of supervisor directory)
# Run with: docker build -f /workspace/group/gassy/supervisor/Dockerfile -t gassy-supervisor /workspace/group/gassy
FROM docker.io/library/golang:1.21 AS builder

WORKDIR /src

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the supervisor source (path is relative to build context /workspace/group/gassy)
COPY cmd/supervisor/main.go ./cmd/supervisor/

# Build the supervisor binary (static build preferred)
RUN CGO_ENABLED=0 go build -ldflags="-extldflags=-static" -o /supervisor ./cmd/supervisor/

# Runtime stage - Fedora with podman
FROM docker.io/fedora:40

# Install podman (static binary preferred, but fedora repos provide it)
RUN dnf install -y --nodocs \
    podman \
    skopeo \
    conmon \
    containernetworking-plugins \
    curl \
    && dnf clean all

# Create nodeapp user with uid 1001
RUN useradd -u 1001 -r -d /home/nodeapp -s /sbin/nologin \
    -c "Node Application" \
    nodeapp && \
    mkdir -m 755 /home/nodeapp && \
    chown nodeapp:nodeapp /home/nodeapp

# Create required directories
RUN mkdir -p /workspace/group && chown nodeapp:nodeapp /workspace/group

# Copy the supervisor binary from builder
COPY --from=builder /supervisor /usr/local/bin/supervisor

# Note: .env file is loaded automatically by supervisor at runtime
# Mount .env at runtime or place it in /workspace/group/gassy/.env inside the container

# Set environment variables for podman remote socket connection
# The socket is mounted at runtime to /run/user/1000/podman/podman.sock
ENV CONTAINER_HOST=unix:///run/user/1000/podman/podman.sock
ENV PODMAN_SOCKET=/run/user/1000/podman/podman.sock

# Set working directory
WORKDIR /workspace/group/gassy

# Switch to non-root user
USER nodeapp

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:9091/health || exit 1

# Entry point runs the supervisor
ENTRYPOINT ["/usr/local/bin/supervisor"]