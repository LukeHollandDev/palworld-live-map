ARG GO_VERSION=1.26.5
ARG NODE_VERSION=24.18.0
ARG PYTHON_VERSION=3.13.5

# The palworld-save-reader release this image ships as its save decoder.
ARG SAVE_READER_VERSION=v0.2.0
ARG SAVE_READER_REVISION=c6560931f407abcbe3398a3fc73840b51bb56974

# Web build
FROM --platform=$BUILDPLATFORM node:${NODE_VERSION}-alpine AS web

WORKDIR /src/web

COPY web/package.json ./
RUN npm install
COPY web ./
RUN npm run build

# Map assets. Generated tiles stay out of Git and are created once while the
# image is built, keeping the final distroless runtime small and quick to boot.
FROM --platform=$BUILDPLATFORM python:${PYTHON_VERSION}-slim-bookworm AS map-assets

WORKDIR /src

COPY tools/map-tiles-requirements.txt tools/generate-map-tiles.py ./tools/
COPY assets/palworld/maps ./assets/palworld/maps
RUN python -m pip install --disable-pip-version-check --no-cache-dir \
      --only-binary=Pillow --requirement tools/map-tiles-requirements.txt \
    && python tools/generate-map-tiles.py --if-needed assets/palworld/maps

# Go build
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY assets ./assets
COPY --from=map-assets /src/assets/palworld/maps ./assets/palworld/maps
COPY cmd ./cmd
COPY internal ./internal
COPY web/embed.go ./web/
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/palworld-live-map ./cmd/palworld-live-map

# Save decoder build
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS reader

ARG TARGETOS
ARG TARGETARCH
ARG SAVE_READER_VERSION
ARG SAVE_READER_REVISION

WORKDIR /reader

COPY patches/palworld-save-reader-v0.2.0-leaderboards.patch /tmp/palworld-save-reader-leaderboards.patch

RUN git clone --branch "${SAVE_READER_VERSION}" --depth 1 \
      https://github.com/LukeHollandDev/palworld-save-reader.git . \
    && test "$(git rev-parse HEAD)" = "${SAVE_READER_REVISION}" \
    && git apply --check /tmp/palworld-save-reader-leaderboards.patch \
    && git apply /tmp/palworld-save-reader-leaderboards.patch \
    && make release-build \
      GOOS="${TARGETOS}" \
      GOARCH="${TARGETARCH}" \
      VERSION="${SAVE_READER_VERSION}+live-map.2" \
      OUTPUT=/out/palworld-save-reader \
    && mkdir -p /out/licenses \
    && cp LICENSE NOTICE /out/licenses/ \
    && cp /tmp/palworld-save-reader-leaderboards.patch /out/licenses/live-map-resolve-v4.patch \
    && mkdir -p /out/licenses/LICENSES \
    && cp LICENSES/Apache-2.0.txt /out/licenses/LICENSES/

# Runtime image
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/palworld-live-map /usr/local/bin/palworld-live-map
COPY LICENSE /licenses/palworld-live-map/LICENSE

# The decoder sits beside the server binary, where savesidecar always looks.
COPY --from=reader /out/palworld-save-reader /usr/local/bin/palworld-save-reader
COPY --from=reader /out/licenses/ /licenses/palworld-save-reader/

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/palworld-live-map", "-healthcheck"]

ENTRYPOINT ["/usr/local/bin/palworld-live-map"]
