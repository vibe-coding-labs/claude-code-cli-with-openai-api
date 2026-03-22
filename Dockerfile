# Multi-stage build for Claude Code CLI with OpenAI API
# Supports multi-platform builds (linux/amd64, linux/arm64)

# Frontend build stage
FROM --platform=$BUILDPLATFORM node:18-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm install
COPY frontend/ .
RUN npm run build

# Backend build stage with cross-compilation support
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS backend-builder
ARG TARGETOS
ARG TARGETARCH

# Install build dependencies and cross-compilation tools
RUN apk add --no-cache git

# Install cross-compilation toolchain for CGO
RUN apk add --no-cache gcc musl-dev sqlite-dev
RUN if [ "$TARGETARCH" = "amd64" ] && [ "$BUILDARCH" != "$TARGETARCH" ]; then \
        apk add --no-cache gcc-x86_64-linux-musl musl-dev; \
        export CC=x86_64-linux-musl-gcc; \
    elif [ "$TARGETARCH" = "arm64" ] && [ "$BUILDARCH" != "$TARGETARCH" ]; then \
        apk add --no-cache gcc-aarch64-linux-musl musl-dev; \
        export CC=aarch64-linux-musl-gcc; \
    fi

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /app/frontend/build ./frontend/build

# Build with proper cross-compilation settings
RUN export CC="$(which ${TARGETARCH}-linux-musl-gcc 2>/dev/null || echo gcc)" && \
    CGO_ENABLED=1 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    CC=${CC} \
    go build -a -installsuffix cgo -ldflags="-w -s -linkmode external -extldflags '-static'" \
    -o claude-with-openai-api .

# Final stage - minimal Alpine image
FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata && \
    addgroup -g 1000 app && \
    adduser -D -u 1000 -G app app

WORKDIR /app

# Copy binary from builder
COPY --from=backend-builder /app/claude-with-openai-api .

# Create data directory with proper permissions
RUN mkdir -p /app/data && \
    chown -R app:app /app

# Switch to non-root user
USER app

# Expose port
EXPOSE 54988

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:54988/health || exit 1

# Use exec form to ensure proper signal handling
CMD ["./claude-with-openai-api", "server"]
