FROM golang:1.26-alpine3.23 AS build

WORKDIR /src
COPY go.mod ./
COPY *.go admin.html api-docs.html openapi.yaml ./
RUN CGO_ENABLED=0 go test ./... && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/homerouter-proxy .

FROM alpine:3.23

RUN addgroup -S -g 10001 proxy && \
    adduser -S -D -H -u 10001 -G proxy proxy && \
    mkdir -p /data && chown proxy:proxy /data

COPY --from=build /out/homerouter-proxy /usr/local/bin/homerouter-proxy
USER 10001:10001
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/homerouter-proxy"]