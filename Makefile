.PHONY: clean local

DIRS := cmd pkg
PKG := `go list ./... | grep -v /vendor/`
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS ?= -X=main.version=$(VERSION)
BINARY := pg-prometheus-exporter
CGO_ENABLED ?= 0

clean:
	rm -f ${BINARY}

tools:
	@go get -u github.com/golang/dep/cmd/dep

fmt:
	@gofmt -l -w -s $(DIRS)

deps:
	@dep ensure -v

all:
	CGO_ENABLED=${CGO_ENABLED} go build -o ${BINARY} -ldflags "$(LDFLAGS)" main.go $^
