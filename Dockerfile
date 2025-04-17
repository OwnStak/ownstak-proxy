FROM --platform=$TARGETPLATFORM alpine:latest AS run

ARG DIST_NAME
ARG OS
ARG ARCH
ENV DIST_NAME=$DIST_NAME

WORKDIR /dist
# Install vips package, so we have all its dependencies but we'll replace it with our own version in next step,
# to make sure we always use the exact same lib as locally.
RUN apk add --no-cache vips libc6-compat

# Copy and keep only the libs for the current platform to reduce the image size
# NOTE: Do this in single RUN so it's in single layer
RUN --mount=type=bind,source=dist/lib,target=/tmp/dist/lib mkdir -p /dist/lib && cp -r /tmp/dist/lib/*${OS}-${ARCH}* /dist/lib/

# Copy the executable
COPY dist/${DIST_NAME}-${OS}-${ARCH} /dist/ownstak-proxy
RUN chmod +x /dist/ownstak-proxy

ENTRYPOINT ["/dist/ownstak-proxy"]