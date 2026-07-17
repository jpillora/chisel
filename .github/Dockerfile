# goreleaser builds the binary and provides it in the build context
FROM alpine:3 AS certs
RUN apk add --no-cache ca-certificates

FROM scratch
LABEL maintainer="dev@jpillora.com"
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
WORKDIR /app
COPY chisel /app/bin
ENTRYPOINT ["/app/bin"]
