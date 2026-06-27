FROM golang:1.24-alpine AS build

ENV GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct
ENV GO111MODULE=on
ENV GOSUMDB=off

RUN sed -i 's/https:\/\/dl-cdn.alpinelinux.org/https:\/\/mirrors.aliyun.com/g' \
    /etc/apk/repositories

WORKDIR /src

RUN apk add --no-cache git ca-certificates build-base

COPY go.mod go.sum ./

RUN echo "下载依赖..." && \
    go mod download || \
    (sleep 10 && go mod download) || \
    (sleep 15 && go mod download) || \
    go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags='-w -s' -o /out/server ./cmd/server && \
    go build -ldflags='-w -s' -o /out/user-service ./cmd/user-service && \
    go build -ldflags='-w -s' -o /out/agent-worker ./cmd/agent-worker && \
    go build -ldflags='-w -s' -o /out/outbox-worker ./cmd/outbox-worker && \
    go build -ldflags='-w -s' -o /out/data-worker ./cmd/data-worker && \
    go build -ldflags='-w -s' -o /out/search-worker ./cmd/search-worker && \
    go build -ldflags='-w -s' -o /out/cleanup-worker ./cmd/cleanup-worker && \
    go build -ldflags='-w -s' -o /out/migrate ./cmd/migrate

FROM alpine:3.18

RUN sed -i 's/https:\/\/dl-cdn.alpinelinux.org/https:\/\/mirrors.aliyun.com/g' \
    /etc/apk/repositories && \
    apk add --no-cache ca-certificates ffmpeg

WORKDIR /app
COPY --from=build /out/server ./server
COPY --from=build /out/user-service ./user-service
COPY --from=build /out/agent-worker ./agent-worker
COPY --from=build /out/outbox-worker ./outbox-worker
COPY --from=build /out/data-worker ./data-worker
COPY --from=build /out/search-worker ./search-worker
COPY --from=build /out/cleanup-worker ./cleanup-worker
COPY --from=build /out/migrate ./migrate
COPY configs ./configs

ENV CONFIG_PATH=/app/configs/config.yaml
ENV HTTP_PORT=8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/v1/health || exit 1
ENTRYPOINT ["/app/server"]
