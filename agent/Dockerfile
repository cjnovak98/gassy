# syntax=docker/dockerfile:1
#
# Multi-stage build for TypeScript agent using Anthropic Agent SDK
#

# =============================================================================
# Stage 1: Go Builder - Build the supervisor and gassy CLI binaries
# =============================================================================
FROM docker.io/library/golang:1.21-alpine AS go-builder

# Install Go dependencies
WORKDIR /build

# Copy go mod files from gassy module root (context is gassy/)
COPY go.mod go.sum* ./

# Download dependencies
RUN go mod download

# Copy all source needed for builds
COPY cmd/supervisor/ ./cmd/supervisor/
COPY cmd/gassy/ ./cmd/gassy/
COPY internal/ ./internal/

# Build the supervisor binary
RUN CGO_ENABLED=0 GOOS=linux go build -o supervisor ./cmd/supervisor/

# Build the gassy CLI
RUN CGO_ENABLED=0 GOOS=linux go build -o gassy ./cmd/gassy/

# =============================================================================
# Stage 2: Builder - Install deps and compile TypeScript
# =============================================================================
FROM docker.io/library/node:22-alpine AS builder

# Install dependencies
WORKDIR /app

# Copy package files first for better layer caching
COPY agent/package.json agent/package-lock.json*.json ./

# Install dependencies
RUN npm install --include=dev

# Copy source code
COPY agent/tsconfig.json ./
COPY agent/src/ ./src/
COPY agent/config/ ./config/

# Compile TypeScript
RUN npm run build

# =============================================================================
# Stage 3: Runtime - Minimal image with compiled output and supervisor
# =============================================================================
FROM docker.io/library/node:22-alpine AS runtime

# Install bash (required by Claude Code CLI) and create symlinks
RUN apk add bash && ln -s /bin/bash /usr/local/bin/bash 2>/dev/null || true

# Security: run as non-root user
RUN addgroup -g 1001 -S nodejs && adduser -S nodeapp -u 1001

WORKDIR /app

# Create artifacts directory for file serving
RUN mkdir -p /app/artifacts

# Copy compiled output from builder
COPY --from=builder --chown=nodeapp:nodejs /app/dist ./dist
COPY --from=builder --chown=nodeapp:nodejs /app/node_modules ./node_modules
COPY --from=builder --chown=nodeapp:nodejs /app/package.json ./
COPY --from=builder --chown=nodeapp:nodejs /app/config ./config

# Copy the supervisor binary from Go builder
COPY --from=go-builder --chown=nodeapp:nodejs /build/supervisor ./supervisor

# Copy the gassy CLI binary
COPY --from=go-builder --chown=nodeapp:nodejs /build/gassy ./gassy

# Ensure /app is owned by nodeapp
RUN chown -R nodeapp:nodejs /app

# Switch to non-root user
USER nodeapp

# Set environment
ENV NODE_ENV=production
ENV PATH="/app:${PATH}"

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD node -e "process.exit(0)" || exit 1

# Entrypoint - use a wrapper that handles both supervisor and agent modes
ENTRYPOINT ["/bin/bash", "-c"]
CMD ["if [ \"$1\" = 'supervisor' ]; then exec /app/supervisor; else exec node dist/index.js \"$@\"; fi"]