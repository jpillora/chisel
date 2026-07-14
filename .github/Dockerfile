ARG GOARCH=amd64
ARG GOARM=
ARG GOMIPS=hardfloat
ARG GO386=

# All build stages explicitly use BUILDPLATFORM so RUN commands execute
# on the host architecture (amd64) even when cross-compiling for other targets.
# This eliminates the need for QEMU entirely — Go's native GOARCH cross-compilation
# handles all target architectures from a single amd64 build host.

FROM --platform=$BUILDPLATFORM docker.io/stagex/pallet-go@sha256:4b7f9fe27d84dd9109fe89c4dab1cefdd66bf2f670817ef95ffd335b84fdf2cb AS deps
ARG CHISEL_VERSION
WORKDIR /app
COPY . .
RUN GO111MODULE=on go mod vendor
RUN test -d vendor && echo "vendor/ created"

FROM --platform=$BUILDPLATFORM docker.io/stagex/pallet-go@sha256:4b7f9fe27d84dd9109fe89c4dab1cefdd66bf2f670817ef95ffd335b84fdf2cb AS check
ARG CHISEL_VERSION
WORKDIR /app
ENV CGO_ENABLED=0
COPY --from=deps /app/ .
RUN go vet -mod=vendor ./...

FROM --platform=$BUILDPLATFORM docker.io/stagex/pallet-go@sha256:4b7f9fe27d84dd9109fe89c4dab1cefdd66bf2f670817ef95ffd335b84fdf2cb AS test
ARG CHISEL_VERSION
WORKDIR /app
ENV CGO_ENABLED=0
COPY --from=check /app/ .
RUN go test -mod=vendor -v ./...

FROM --platform=$BUILDPLATFORM docker.io/stagex/pallet-go@sha256:4b7f9fe27d84dd9109fe89c4dab1cefdd66bf2f670817ef95ffd335b84fdf2cb AS build
ARG CHISEL_VERSION
ARG GOARCH=amd64
ARG GOARM=
ARG GOMIPS=hardfloat
ARG GO386=
WORKDIR /app
ENV CGO_ENABLED=0
ENV GOARCH=${GOARCH}
ENV GOARM=${GOARM}
ENV GOMIPS=${GOMIPS}
ENV GO386=${GO386}
COPY --from=test /app/ .
RUN --network=none \
    go build -mod=vendor -trimpath \
        -ldflags="-s -w -X github.com/jpillora/chisel/share.BuildVersion=${CHISEL_VERSION#v}" \
        -o /chisel .

FROM scratch
ARG CHISEL_VERSION
COPY --from=build /chisel /chisel
ENTRYPOINT ["/chisel"]
