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

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/palworld-live-map /usr/local/bin/palworld-live-map
COPY LICENSE /licenses/palworld-live-map/LICENSE
COPY LICENSING.md /licenses/palworld-live-map/LICENSING.md
COPY LICENSES/React-MIT.txt /licenses/react/React-MIT.txt

LABEL org.opencontainers.image.licenses="MIT AND LicenseRef-Pocketpair-Proprietary"

USER nonroot:nonroot
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/palworld-live-map", "-healthcheck"]

ENTRYPOINT ["/usr/local/bin/palworld-live-map"]
