
.PHONY: all
all: fmt build

.PHONY: build
build: proxypool

.PHONY: fmt
fmt:
	@go fmt ./...

.PHONY: proxypool
proxypool:
	@[ ! -f go.mod ] && go mod init $@ || exit 0
	@[ ! -d bin ] && mkdir -p bin || exit 0
	@go fmt ./...
	@go build -ldflags "-w -s" -o bin/$@ main.go

.PHONY: clean
clean:
	@rm -rf bin

.PHONY: distclean
distclean:
	@rm -rf bin
	@go clean --modcache
