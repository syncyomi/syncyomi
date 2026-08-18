# build web
FROM --platform=$BUILDPLATFORM node:24-alpine AS web-builder

WORKDIR /web

COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./

RUN corepack enable && corepack prepare --activate

RUN pnpm install --frozen-lockfile

COPY web/ ./

RUN pnpm run build

# build app
FROM --platform=$BUILDPLATFORM golang:1.26-alpine3.23 AS app-builder

ARG VERSION=dev
ARG REVISION=dev
ARG BUILDTIME
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

RUN apk add --no-cache git make build-base tzdata

ENV SERVICE=syncyomi

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . ./

# after COPY . ./ so the real dist replaces the .gitkeep placeholder web/dist holds in git
COPY --from=web-builder /web/dist ./web/dist

# Cross-compile natively on the build host (CGO disabled) instead of under QEMU.
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} \
    go build -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${REVISION} -X main.date=${BUILDTIME}" -o bin/syncyomi main.go

# build final image
FROM alpine:3.24

LABEL org.opencontainers.image.source="https://github.com/SyncYomi/SyncYomi"

ENV HOME="/config" \
    XDG_CONFIG_HOME="/config" \
    XDG_DATA_HOME="/config"

RUN apk add --no-cache ca-certificates curl tzdata jq && apk upgrade --no-cache

WORKDIR /app

VOLUME /config

COPY --from=app-builder /src/bin/syncyomi /usr/local/bin/

EXPOSE 8282

ENTRYPOINT ["/usr/local/bin/syncyomi", "--config", "/config"]
