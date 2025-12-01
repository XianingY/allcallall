FROM golang:1.22-alpine AS build

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
    go build -ldflags='-w -s' -o /out/server ./cmd/server

FROM alpine:3.18

RUN sed -i 's/https:\/\/dl-cdn.alpinelinux.org/https:\/\/mirrors.aliyun.com/g' \
    /etc/apk/repositories && \
    apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=build /out/server ./server
COPY configs ./configs

ENV CONFIG_PATH=/app/configs/config.yaml
ENV HTTP_PORT=8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/v1/health || exit 1
ENTRYPOINT ["/app/server"]
