# syntax=docker/dockerfile:1
FROM golang:1.24-alpine AS build
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/overmynd ./cmd/overmynd

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/overmynd /overmynd
ENV UNDERMYND_CONFIG=/config UNDERMYND_DATA=/data PUID=99 PGID=100
EXPOSE 8080 8081
VOLUME ["/config"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["/overmynd", "healthcheck"]
ENTRYPOINT ["/overmynd"]
