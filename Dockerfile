# syntax=docker/dockerfile:1.7
FROM node:22-bookworm-slim AS web-build
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY frontend/ ./
COPY webui/ /src/webui/
RUN npm run build

FROM golang:1.27-bookworm AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY webui/ ./webui/
COPY --from=web-build /src/webui/dist/ ./webui/dist/
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_TIME=unknown
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /out/muni ./cmd/muni

FROM debian:bookworm-slim
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates chromium curl fonts-noto-cjk tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && update-ca-certificates
COPY --from=go-build /out/muni /usr/local/bin/muni
RUN groupadd --system --gid 65532 muni && useradd --system --uid 65532 --gid 65532 --no-create-home --home-dir /nonexistent muni
USER 65532:65532
EXPOSE 8080
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD ["curl", "--fail", "--silent", "http://127.0.0.1:8080/healthz"]
ENTRYPOINT ["/usr/local/bin/muni"]
