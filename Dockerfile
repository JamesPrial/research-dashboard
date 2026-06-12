# Stage 1: Build the Go binary
FROM golang:1.25-alpine AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG TARGETVARIANT

WORKDIR /build

COPY go.mod ./
COPY main.go ./
COPY internal/ internal/
COPY static/ static/
COPY research-config/ research-config/

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    GOARM=${TARGETVARIANT#v} \
    go build -o research-dashboard .

# Stage 2: Runtime with Node.js (required for Claude CLI)
FROM node:20-slim

# Install gosu for privilege dropping in entrypoint.
RUN apt-get update \
    && apt-get install -y --no-install-recommends gosu \
    && rm -rf /var/lib/apt/lists/* \
    && gosu nobody true

RUN npm install -g @anthropic-ai/claude-code

RUN useradd -m -s /bin/bash researcher \
    && mkdir -p /research \
    && chown researcher:researcher /research

COPY --from=builder /build/research-dashboard /usr/local/bin/research-dashboard
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

WORKDIR /research

EXPOSE 8420

# Node provides fetch (no curl/wget in slim images). PORT is set by compose;
# plain docker run uses the 8420 default.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD node -e "fetch('http://localhost:'+(process.env.PORT||8420)+'/research').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["--cwd", "/research", "--claude-path", "claude"]
