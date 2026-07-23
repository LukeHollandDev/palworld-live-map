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

# Build the GPL decoder-only helper for the target platform. Running this
# stage on TARGETPLATFORM keeps the C++ build native under Buildx/QEMU and
# produces both amd64 and arm64 helpers from the same corresponding source.
FROM golang:${GO_VERSION}-bookworm AS decoder

WORKDIR /src

COPY third_party/palworld-save-decode ./
RUN mkdir -p /out && make OUTPUT=/out/palworld-save-decode

# Cross-compile the otherwise CGO-free Go application for the target platform.
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Download dependencies separately to preserve the module cache when source files change.
COPY go.mod ./
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

# The decoder helper needs the platform C++ runtime even though the Go build
# itself does not use cgo.
FROM gcr.io/distroless/cc-debian12:nonroot

COPY --from=build /out/palworld-live-map /usr/local/bin/palworld-live-map
COPY --from=decoder /out/palworld-save-decode /usr/local/bin/palworld-save-decode
# Keep the exact corresponding helper source in every binary image as well as
# in the tagged repository source selected by the OCI revision label.
COPY --from=decoder /src /usr/src/palworld-save-decode
COPY LICENSE /licenses/palworld-live-map/LICENSE
COPY LICENSING.md /licenses/palworld-live-map/LICENSING.md
COPY LICENSES/React-MIT.txt /licenses/react/React-MIT.txt
COPY --from=build /src/internal/savegame/LICENSE /licenses/palhelm/LICENSE
COPY --from=build /src/internal/savegame/NOTICE /licenses/palhelm/NOTICE
COPY --from=decoder /src/LICENSES/GPL-3.0-or-later.txt /licenses/palworld-save-decode/GPL-3.0-or-later.txt
COPY --from=decoder /src/LICENSES/MIT.txt /licenses/palworld-save-decode/SIMDe-MIT.txt
COPY --from=decoder /src/LICENSES/CC0-1.0.txt /licenses/palworld-save-decode/SIMDe-CC0-1.0.txt
COPY --from=decoder /src/NOTICE /licenses/palworld-save-decode/NOTICE

LABEL org.opencontainers.image.licenses="MIT AND Apache-2.0 AND GPL-3.0-or-later AND CC0-1.0 AND LicenseRef-Pocketpair-Proprietary"

USER nonroot:nonroot
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/palworld-live-map", "-healthcheck"]

ENTRYPOINT ["/usr/local/bin/palworld-live-map"]
