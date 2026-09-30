# syntax=docker/dockerfile:1
# Build natively on the build host and cross-compile (no QEMU needed).
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS build
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /src
COPY go.mod ./
COPY . .
ARG TARGETOS TARGETARCH VERSION=dev
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/familydash ./cmd/familydash
# scratch has no /tmp; the things3 CLI falls back to it when /data isn't mounted.
# Copied as part of a root folder so the directory keeps its 1777 mode.
RUN mkdir -p /out/root/tmp && chmod 1777 /out/root/tmp

# things3 CLI (unofficial Things Cloud client, MIT) for the to-do card.
# Static musl binary, cross-compiled with zig – same "no QEMU" approach as above.
# Pinned to a release tag and its commit; bump both together.
FROM --platform=$BUILDPLATFORM rust:1.98-trixie AS things
ARG THINGS3_VERSION=v0.10.0
ARG THINGS3_COMMIT=bc486c1bcbe0aebfb06c8742703b8b62bd8e9c36
RUN apt-get update && apt-get install -y --no-install-recommends python3-pip \
    && rm -rf /var/lib/apt/lists/* \
    && pip3 install --no-cache-dir --break-system-packages ziglang==0.16.0 cargo-zigbuild==0.23.4 \
    && rustup target add x86_64-unknown-linux-musl aarch64-unknown-linux-musl
WORKDIR /src
RUN git clone --depth 1 --branch "$THINGS3_VERSION" https://github.com/evanpurkhiser/things3-cloud . \
    && test "$(git rev-parse HEAD)" = "$THINGS3_COMMIT"
RUN cargo fetch --locked
ARG TARGETARCH
RUN case "$TARGETARCH" in \
      amd64) T=x86_64-unknown-linux-musl ;; \
      arm64) T=aarch64-unknown-linux-musl ;; \
      *) echo "unsupported arch $TARGETARCH" >&2; exit 1 ;; \
    esac \
    && cargo zigbuild --release --locked --target "$T" \
    && mkdir -p /out && cp "target/$T/release/things3" /out/things3

# ~20 MB final image: the static binaries, CA certs and time zones (the
# things3 CLI needs them to know what "today" is).
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /out/root/ /
COPY --from=build /out/familydash /familydash
COPY --from=things /out/things3 /things3
# state: only the Things sync cache in /data/things (optional mount)
# runs as nobody:users (Unraid default)
USER 99:100
ENV TZ=Europe/Berlin LISTEN_ADDR=:8080
EXPOSE 8080
HEALTHCHECK --interval=60s --timeout=5s --start-period=10s CMD ["/familydash", "-healthcheck"]
ENTRYPOINT ["/familydash"]
