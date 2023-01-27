# build stage
FROM golang:1.19 as build
LABEL maintainer="dev@jpillora.com"
ENV CGO_ENABLED 0
ADD . /src
WORKDIR /src
RUN go mod download
RUN go build \
    -ldflags "-X github.com/jpillora/chisel/share.BuildVersion=$(git describe --abbrev=0 --tags)" \
    -o chisel
# run stage
FROM scratch
COPY --from=alpine:latest /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
WORKDIR /app
COPY --from=build /src/chisel /app/chisel
ENTRYPOINT ["/app/chisel"]