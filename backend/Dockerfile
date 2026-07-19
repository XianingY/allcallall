FROM golang:1.24.13-alpine AS build

ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}

WORKDIR /src

COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
    echo "下载依赖..." && \
    go mod download || \
    (sleep 10 && go mod download) || \
    (sleep 15 && go mod download) || \
    go mod download

COPY . .

ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} \
    go build -ldflags='-w -s' -o /out/server ./cmd/server && \
    go build -ldflags='-w -s' -o /out/user-service ./cmd/user-service && \
    go build -ldflags='-w -s' -o /out/agent-worker ./cmd/agent-worker && \
    go build -ldflags='-w -s' -o /out/outbox-worker ./cmd/outbox-worker && \
    go build -ldflags='-w -s' -o /out/data-worker ./cmd/data-worker && \
    go build -ldflags='-w -s' -o /out/search-worker ./cmd/search-worker && \
    go build -ldflags='-w -s' -o /out/cleanup-worker ./cmd/cleanup-worker && \
    go build -ldflags='-w -s' -o /out/sandbox-service ./cmd/sandbox-service && \
    go build -ldflags='-w -s' -o /out/migrate ./cmd/migrate && \
    go build -ldflags='-w -s' -o /out/interview-seed ./cmd/interview-seed

FROM alpine:3.22

RUN apk upgrade --no-cache && \
    apk add --no-cache ca-certificates ffmpeg

WORKDIR /app
COPY --from=build /out/server ./server
COPY --from=build /out/user-service ./user-service
COPY --from=build /out/agent-worker ./agent-worker
COPY --from=build /out/outbox-worker ./outbox-worker
COPY --from=build /out/data-worker ./data-worker
COPY --from=build /out/search-worker ./search-worker
COPY --from=build /out/cleanup-worker ./cleanup-worker
COPY --from=build /out/sandbox-service ./sandbox-service
COPY --from=build /out/migrate ./migrate
COPY --from=build /out/interview-seed ./interview-seed
COPY configs ./configs
COPY migrations ./migrations

ENV CONFIG_PATH=/app/configs/config.yaml
ENV HTTP_PORT=8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/v1/health || exit 1
ENTRYPOINT ["/app/server"]
