# build stage
FROM golang:alpine AS build-env
LABEL maintainer="dev@jpillora.com"
RUN apk update
RUN apk add git ca-certificates
RUN mkdir /newroot/ && tar c /usr/share/ca-certificates/ /etc/ssl/ /etc/ca-certificates.conf | tar xC /newroot/
ENV CGO_ENABLED 0
ADD . /src
WORKDIR /src
RUN go build \
    -mod vendor \
    -ldflags "-X github.com/jpillora/chisel/share.BuildVersion=$(git describe --abbrev=0 --tags)" \
    -o chisel

# container stage
FROM scratch
COPY --from=build-env /newroot/ /
COPY --from=build-env /src/chisel /app/chisel
WORKDIR /app
ENTRYPOINT ["/app/chisel"]