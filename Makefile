VERSION=$(shell git describe --abbrev=0 --tags)
BUILD=$(shell git rev-parse HEAD)
DIRBASE=./build
DIR=${DIRBASE}/${VERSION}/${BUILD}/bin

LDFLAGS=-ldflags "-s -w ${XBUILD} -buildid=${BUILD} -X github.com/jpillora/chisel/share.BuildVersion=${VERSION}"

GOFILES=`go list ./...`
GOFILESNOTEST=`go list ./... | grep -v test`

all:
	@goreleaser build --skip-validate --single-target --config .github/goreleaser.yml

freebsd: lint ${DIR}
	env CGO_ENABLED=0 GOOS=freebsd GOARCH=amd64 go build -trimpath ${LDFLAGS} ${GCFLAGS} ${ASMFLAGS} -o ${DIR}/chisel-freebsd_amd64 .

linux: lint ${DIR}
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath ${LDFLAGS} ${GCFLAGS} ${ASMFLAGS} -o ${DIR}/chisel-linux_amd64 .

windows: lint ${DIR}
	env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath ${LDFLAGS} ${GCFLAGS} ${ASMFLAGS} -o ${DIR}/chisel-windows_amd64 .

windows_binaries: lint ${DIR}
	env CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc-win32 go build -buildmode=c-shared -trimpath ${LDFLAGS} ${GCFLAGS} ${ASMFLAGS} -o ${DIR}/chisel.x64.dll .
	env CGO_ENABLED=1 GOOS=windows GOARCH=386 CC=i686-w64-mingw32-gcc-win32 go build -buildmode=c-shared -trimpath ${LDFLAGS} ${GCFLAGS} ${ASMFLAGS} -o ${DIR}/chisel.x86.dll .

SLIVER_ALIAS ?= $(HOME)/.sliver-client/aliases/chisel

sliver: windows_binaries alias ## Build + install the Sliver alias

alias: ## Copy alias.json and the shared libs into the Sliver aliases dir
	@mkdir -p ${SLIVER_ALIAS}
	cp alias.json ${DIR}/chisel.x64.dll ${DIR}/chisel.x86.dll ${SLIVER_ALIAS}/
	@echo "Installed Sliver alias -> ${SLIVER_ALIAS}"

darwin: ${DIR}
	env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath ${LDFLAGS} ${GCFLAGS} ${ASMFLAGS} -o ${DIR}/chisel-darwin_amd64 .

docker:
	@docker build .

dep: ## Get the dependencies
	@go install github.com/goreleaser/goreleaser/v2@latest
	@go install github.com/boumenot/gocover-cobertura@latest
	@go mod tidy

lint: ## Lint the files
	@go fmt ${GOFILES}
	@go vet ${GOFILESNOTEST}

${DIR}:
	mkdir -p ${DIR}

test: ${DIR} ## Run unit tests
	@go test -coverprofile=${DIR}/coverage.out -race -short ${GOFILESNOTEST}
	@go tool cover -html=${DIR}/coverage.out -o ${DIR}/coverage.html
	@gocover-cobertura < ${DIR}/coverage.out > ${DIR}/coverage.xml

release: lint test
	goreleaser release --config .github/goreleaser.yml

clean:
	rm -rf ${DIRBASE}/*

.PHONY: all freebsd linux windows windows_binaries sliver alias docker dep lint test release clean