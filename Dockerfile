ARG GO_VERSION=1.26.5
ARG NODE_VERSION=24

# Compile the React application into static assets for the Go embed package.
FROM node:${NODE_VERSION}-alpine AS web

WORKDIR /src/web

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

# Build a static binary for the requested target platform.
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Download dependencies separately to preserve the module cache when source files change.
COPY go.mod ./
RUN go mod download

COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/palworld-live-map ./cmd/palworld-live-map

# The runtime image contains only the application and CA certificates.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/palworld-live-map /palworld-live-map

USER nonroot:nonroot
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/palworld-live-map", "-healthcheck"]

ENTRYPOINT ["/palworld-live-map"]
