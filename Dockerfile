ARG GO_VERSION=1.26.5
ARG NODE_VERSION=24

# Compile the React application into static assets for the Go embed package.
FROM --platform=$BUILDPLATFORM node:${NODE_VERSION}-alpine AS web

WORKDIR /src/web

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/index.html web/tsconfig.json web/vite.config.ts ./
COPY web/public ./public
COPY web/src ./src
RUN npm run build

# Build against Debian's runtime ABI because purego's dynamic-loader bridge
# causes the otherwise CGO-free binary to retain a platform interpreter.
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Download dependencies separately to preserve the module cache when source files change.
COPY go.mod go.sum ./
RUN go mod download

COPY assets/embed.go ./assets/
COPY assets/map/manifest.json assets/map/*.jpg ./assets/map/
COPY assets/landmarks/manifest.json ./assets/landmarks/
COPY cmd ./cmd
COPY internal ./internal
COPY web/embed.go ./web/
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/palworld-live-map ./cmd/palworld-live-map

# purego's dynamic-loader bridge and the operator-provided Oodle runtime need
# the platform C/C++ runtime even though the Go build itself does not use cgo.
FROM gcr.io/distroless/cc-debian12:nonroot

COPY --from=build /out/palworld-live-map /palworld-live-map
COPY LICENSE /licenses/palworld-live-map/LICENSE
COPY --from=build /src/internal/savegame/LICENSE /licenses/palhelm/LICENSE
COPY --from=build /src/internal/savegame/NOTICE /licenses/palhelm/NOTICE
COPY --from=build /src/internal/savegame/THIRD_PARTY_NOTICES.md /licenses/savegame/THIRD_PARTY_NOTICES.md
COPY --from=build /go/pkg/mod/github.com/ebitengine/purego@v0.8.4/LICENSE /licenses/purego/LICENSE

USER nonroot:nonroot
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/palworld-live-map", "-healthcheck"]

ENTRYPOINT ["/palworld-live-map"]
