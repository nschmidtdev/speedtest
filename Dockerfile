FROM golang:1.26-alpine AS build
WORKDIR /src

COPY . .
RUN go mod download
RUN go mod tidy

ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/speedtest ./cmd/speedtest/

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S speedtest \
    && adduser -S -G speedtest -h /data speedtest \
    && mkdir -p /data \
    && chown speedtest:speedtest /data

COPY --from=build /out/speedtest /usr/local/bin/speedtest

ENV SPEEDTEST_PORT=8080 \
    SPEEDTEST_DB=/data/speedtest.db \
    TZ=Europe/Berlin

USER speedtest:speedtest
WORKDIR /data
VOLUME ["/data"]
EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/api/health >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/speedtest"]