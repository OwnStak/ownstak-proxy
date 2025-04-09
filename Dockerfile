FROM --platform=$BUILDPLATFORM alpine:latest AS run

ARG DIST_NAME
ARG OS
ARG ARCH
ENV DIST_NAME=$DIST_NAME

WORKDIR /
COPY /dist/${DIST_NAME}-${OS}-${ARCH} /ownstak-proxy
RUN chmod +x /ownstak-proxy
ENTRYPOINT ["/ownstak-proxy"]