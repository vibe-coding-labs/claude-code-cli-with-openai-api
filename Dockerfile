# Multi-stage build for Claude Code CLI with OpenAI API
FROM node:18-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm install
COPY frontend/ .
RUN npm run build

FROM golang:1.24-alpine AS backend-builder
RUN apk add --no-cache gcc musl-dev sqlite-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /app/frontend/build ./frontend/build
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags="-w -s" -o claude-with-openai-api .

FROM alpine:latest
RUN apk --no-cache add ca-certificates sqlite tzdata && \
    addgroup -g 1000 app && \
    adduser -D -u 1000 -G app app

WORKDIR /app

# Copy binary
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
