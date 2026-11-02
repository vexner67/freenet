# One image per service, all from this file:
#
#   docker build --build-arg SERVICE=user -t freenet-user .
#   docker build --build-arg SERVICE=gateway --target gateway -t freenet-gateway .
#   docker build --build-arg SERVICE=federation --target federation -t freenet-federation .
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

RUN test -n "$SERVICE" || (echo "pass --build-arg SERVICE=<auth|user|post|media|notification|message|club|federation|gateway>" && exit 1)

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

# The federation service cuts the first frames of videos of other servers
# with ffmpeg: a static one with only what that takes, since it reads
# files of strangers. Built on the target platform, once per version.
FROM alpine:3.22 AS ffmpeg
ARG FFMPEG_VERSION=7.1.1
ARG FFMPEG_SHA256=733984395e0dbbe5c046abda2dc49a5544e7e0e1e2366bba849222ae9e3a03b1
RUN apk add --no-cache build-base nasm
WORKDIR /src
RUN wget -q https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz \
    && echo "${FFMPEG_SHA256}  ffmpeg-${FFMPEG_VERSION}.tar.xz" | sha256sum -c - \
    && tar -xJf ffmpeg-${FFMPEG_VERSION}.tar.xz --strip-components=1
RUN ./configure --disable-everything --disable-autodetect --disable-network --disable-doc --disable-debug \
        --disable-ffplay --disable-ffprobe --enable-static --disable-shared --extra-ldflags=-static \
        --enable-protocol=pipe \
        --enable-demuxer=mov,matroska \
        --enable-parser=h264,hevc,mpeg4video,vp8,vp9 \
        --enable-decoder=h264,hevc,mpeg4,vp8,vp9 \
        --enable-filter=scale,format,buffer,buffersink \
        --enable-encoder=mjpeg --enable-muxer=image2 \
    && make -j"$(nproc)" ffmpeg \
    && strip ffmpeg

FROM runtime AS federation
COPY --from=ffmpeg /src/ffmpeg /ffmpeg
ENV FFMPEG_PATH=/ffmpeg

FROM runtime
