# ko needs an OCI base image rather than Dockerfile stages. Build one local,
# architecture-neutral base containing only the system trust store, then let ko
# layer each statically linked target binary onto it without QEMU.
FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS certs

FROM scratch
LABEL maintainer="dev@jpillora.com"
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt
WORKDIR /app
