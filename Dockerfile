# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Build stage
# ---------------------------------------------------------------------------
FROM golang:1.24-alpine AS build

WORKDIR /src

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags "-s -w -X github.com/EisonMe/zypanel/internal/version.Version=${VERSION} -X github.com/EisonMe/zypanel/internal/version.Commit=${COMMIT} -X github.com/EisonMe/zypanel/internal/version.Date=${DATE}" \
    -o /out/zypanel ./cmd/zypanel

# ---------------------------------------------------------------------------
# Runtime stage
# ---------------------------------------------------------------------------
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata docker-cli iproute2 curl \
    && addgroup -S zypanel 2>/dev/null || true \
    && adduser -S -G zypanel zypanel 2>/dev/null || true \
    && mkdir -p /data \
    && chown -R zypanel:zypanel /data

ENV TZ=Asia/Shanghai \
    ZYPANEL_ADDR=:8090 \
    ZYPANEL_DATA_DIR=/data

WORKDIR /app
COPY --from=build /out/zypanel /usr/local/bin/zypanel

# The panel needs read access to the Docker socket when that feature is on.
# The container therefore runs as root by default; switch to a dedicated user
# and add it to the host docker group if your setup allows it.
USER root

EXPOSE 8090

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD curl -fsS http://127.0.0.1:8090/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/zypanel"]
