# Stage 1: Build the Go binary
FROM golang:1.25-alpine AS builder

WORKDIR /build

COPY go.mod ./
COPY main.go main_test.go ./
COPY internal/ internal/
COPY static/ static/
COPY research-config/ research-config/

RUN CGO_ENABLED=0 GOOS=linux go build -o research-dashboard .

# Stage 2: Runtime with Node.js (required for Claude CLI)
FROM node:20-slim

RUN npm install -g @anthropic-ai/claude-code

RUN useradd -m -s /bin/bash researcher \
    && mkdir -p /research \
    && chown researcher:researcher /research

COPY --from=builder /build/research-dashboard /usr/local/bin/research-dashboard

USER researcher
WORKDIR /research

EXPOSE 8420

ENTRYPOINT ["research-dashboard", "--cwd", "/research", "--claude-path", "claude"]
