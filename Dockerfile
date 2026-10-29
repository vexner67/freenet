# One image per service, all from this file:
#
#   docker build --build-arg SERVICE=user -t freenet-user .
#   docker build --build-arg SERVICE=gateway --target gateway -t freenet-gateway .
#
# The services share the go.work workspace and pkg/*, so the build context
# is the repository root. Go modules and the build cache live in BuildKit
# cache mounts: a change in one service rebuilds only what it touches.

ARG GO_VERSION=1.27.1

# The build runs on the builder's own platform and Go cross-compiles for
# the target: an amd64 server's images build fast on an arm64 Mac too.
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build
ARG SERVICE
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src

RUN test -n "$SERVICE" || (echo "pass --build-arg SERVICE=<auth|user|post|media|notification|message|club|gateway>" && exit 1)

# Static binary without debug info; -trimpath keeps local paths out of it.
RUN --mount=type=bind,target=. \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/service ./${SERVICE}/cmd/${SERVICE}

RUN mkdir /out/data

# Distroless: no shell or package manager, runs as nonroot (65532). /data
# belongs to it, for the media files on a volume.
FROM gcr.io/distroless/static-debian13:nonroot AS runtime
COPY --from=build /out/service /service
COPY --from=build --chown=nonroot:nonroot /out/data /data
# Not the image's /home/nonroot (mode 700): compose may run the service as
# another user, who could not even look for an optional .env there.
WORKDIR /
ENTRYPOINT ["/service"]
# The binary asks the running service itself: no shell or curl here. Often
# while starting, so that dependants start soon; then twice a minute —
# every check starts the binary and makes an mTLS handshake, which a
# small server feels.
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --start-interval=1s \
    CMD ["/service", "-healthcheck"]

# The gateway also serves the website on the same origin.
FROM runtime AS gateway
COPY web /web
ENV WEB_DIR=/web

FROM runtime
