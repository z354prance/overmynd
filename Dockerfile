FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./

RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/overmynd \
    ./cmd/overmynd

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 1000 overmynd \
    && adduser -S -D -H -u 1000 -G overmynd overmynd \
    && mkdir -p /config \
    && chown overmynd:overmynd /config

COPY --from=builder /out/overmynd /usr/local/bin/overmynd

USER overmynd

VOLUME ["/config"]

EXPOSE 8080

ENV OVERMYND_ADDR=:8080
ENV OVERMYND_CONFIG_DIR=/config

HEALTHCHECK \
    --interval=30s \
    --timeout=5s \
    --start-period=5s \
    --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/health || exit 1

ENTRYPOINT ["/usr/local/bin/overmynd"]
