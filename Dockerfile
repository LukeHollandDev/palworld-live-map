ARG GO_VERSION=1.26.5
ARG NODE_VERSION=24

# Assemble a buildable Corresponding Source tree without relying on a broad
# COPY that could capture local save data, credentials, VCS metadata, or
# generated outputs.
FROM scratch AS corresponding-source

COPY .dockerignore .env.example .gitignore CONTRIBUTING.md DEVELOPMENT.md Dockerfile LICENSE LICENSING.md Makefile NOTICE README.md ROADMAP-1.0.md SECURITY.md compose.yml /source/
COPY go.* /source/
COPY LICENSES /source/LICENSES
COPY .github /source/.github
COPY assets /source/assets
COPY cmd /source/cmd
COPY docs /source/docs
COPY internal /source/internal
COPY tools /source/tools
COPY web /source/web

# Compile the React application into static assets for the Go embed package.
FROM --platform=$BUILDPLATFORM node:${NODE_VERSION}-alpine AS web

WORKDIR /src/web

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/index.html web/tsconfig.json web/vite.config.ts ./
COPY web/public ./public
COPY web/src ./src
RUN npm run build

# Cross-compile the CGO-free Go application for the target platform.
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Download dependencies separately to preserve the module cache when source
# files change.
COPY go.* ./
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
COPY --from=corresponding-source /source /usr/src/palworld-live-map
COPY LICENSE /licenses/palworld-live-map/LICENSE
COPY LICENSES/GPL-3.0-or-later.txt /licenses/palworld-live-map/GPL-3.0-or-later.txt
COPY LICENSING.md /licenses/palworld-live-map/LICENSING.md
COPY NOTICE /licenses/palworld-live-map/NOTICE
COPY LICENSES/React-MIT.txt /licenses/react/React-MIT.txt
COPY --from=build /src/internal/savegame/LICENSE /licenses/palhelm/LICENSE
COPY --from=build /src/internal/savegame/NOTICE /licenses/palhelm/NOTICE
COPY --from=build /src/internal/palsav/LICENSE /licenses/palsav/GPL-3.0-or-later.txt
COPY --from=build /src/internal/palsav/LICENSES/Apache-2.0.txt /licenses/palsav/Apache-2.0.txt
COPY --from=build /src/internal/palsav/NOTICE /licenses/palsav/NOTICE

LABEL org.opencontainers.image.licenses="GPL-3.0-or-later AND Apache-2.0 AND MIT AND LicenseRef-Pocketpair-Proprietary"

USER nonroot:nonroot
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/palworld-live-map", "-healthcheck"]

ENTRYPOINT ["/usr/local/bin/palworld-live-map"]
