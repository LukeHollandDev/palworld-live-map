ARG GO_VERSION=1.26.5
ARG NODE_VERSION=24

# Web build
FROM --platform=$BUILDPLATFORM node:${NODE_VERSION}-alpine AS web

WORKDIR /src/web

COPY web/package.json ./
RUN npm install
COPY web ./
RUN npm run build

# Go build
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY assets ./assets
COPY cmd ./cmd
COPY internal ./internal
COPY web/embed.go ./web/
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/palworld-live-map ./cmd/palworld-live-map

# Runtime image
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/palworld-live-map /usr/local/bin/palworld-live-map
COPY LICENSE /licenses/palworld-live-map/LICENSE

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/palworld-live-map", "-healthcheck"]

ENTRYPOINT ["/usr/local/bin/palworld-live-map"]
