# ko needs an OCI base image rather than Dockerfile stages. CI publishes this
# CA-only scratch image to a local six-platform registry, then ko layers each
# statically linked target binary onto its matching platform without QEMU.
FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS certs

FROM scratch
LABEL maintainer="dev@jpillora.com"
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt
WORKDIR /app
