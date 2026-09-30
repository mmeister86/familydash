# syntax=docker/dockerfile:1
# Build natively on the build host and cross-compile (no QEMU needed).
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS build
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY go.mod ./
COPY . .
ARG TARGETOS TARGETARCH VERSION=dev
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/familydash ./cmd/familydash \
 && mkdir -p /out/data

# ~8 MB final image: just the static binary + CA certs.
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/familydash /familydash
# 99:100 = nobody:users, the Unraid default owner of /mnt/user/appdata
COPY --from=build --chown=99:100 /out/data /data
USER 99:100
ENV TZ=Europe/Berlin DATA_DIR=/data LISTEN_ADDR=:8080
EXPOSE 8080
VOLUME /data
HEALTHCHECK --interval=60s --timeout=5s --start-period=10s CMD ["/familydash", "-healthcheck"]
ENTRYPOINT ["/familydash"]
