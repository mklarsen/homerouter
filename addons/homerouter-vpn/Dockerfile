FROM golang:1.26-alpine3.23 AS build

ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY web ./web
RUN CGO_ENABLED=0 go vet ./... && CGO_ENABLED=0 go test ./... && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/homerouter-vpn .

FROM alpine:3.23

RUN apk add --no-cache wireguard-tools iproute2 && \
    addgroup -S -g 10002 vpn && \
    adduser -S -D -H -u 10002 -G vpn vpn && \
    mkdir -p /data && chown vpn:vpn /data

COPY --from=build /out/homerouter-vpn /usr/local/bin/homerouter-vpn
USER 10002:10002
EXPOSE 8081
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8081/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/homerouter-vpn"]
