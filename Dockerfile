# We use Ubuntu minimal 22.04 as base image, because it already has glib included in base image and VIPS works out of the box with it.
# Alpine is based on musl libc, and even though we install glib and deps: vips libc6-compat gcompat musl-dev libstdc++ glibc,
# VIPS still outputs warnings that some functions are not available. Alpine image is then quite big with all those deps installed.
# We might try glib based alpine image: https://hub.docker.com/r/frolvlad/alpine-glibc/
FROM --platform=$TARGETPLATFORM ubuntu:22.04 AS run

ARG DIST_NAME
ARG OS
ARG ARCH
ENV DIST_NAME=$DIST_NAME
ENV DEBIAN_FRONTEND=noninteractive
ENV MALLOC_ARENA_MAX=2
WORKDIR /dist

# Update root CA certs, clean up the layer
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && apt-get clean autoclean && apt-get autoremove -y && apt-get clean && rm -rf /var/lib/apt/lists/*

# Copy and keep only the libs for the current platform to reduce the image size
# NOTE: Do this in single RUN so it's in single layer
RUN --mount=type=bind,source=dist/lib,target=/tmp/dist/lib mkdir -p /dist/lib && cp -r /tmp/dist/lib/*${OS}-${ARCH}* /dist/lib/

# Copy the executable
COPY dist/${DIST_NAME}-${OS}-${ARCH} /dist/ownstak-proxy
RUN chmod +x /dist/ownstak-proxy

ENTRYPOINT ["/dist/ownstak-proxy"]