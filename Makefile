BUILDDIR ?= $(CURDIR)/build
TOOLS_DIR := tools

GO_BIN := ${GOPATH}/bin

DOCKER := $(shell which docker)
CUR_DIR := $(shell pwd)
MOCKS_DIR=$(CUR_DIR)/testutil/mocks
MOCKGEN_REPO=github.com/golang/mock/mockgen
MOCKGEN_VERSION=v1.6.0
MOCKGEN_CMD=go run ${MOCKGEN_REPO}@${MOCKGEN_VERSION}

ldflags := $(LDFLAGS)

PACKAGES_E2E=$(shell go list ./... | grep '/itest')

ifeq ($(LINK_STATICALLY),true)
	ldflags += -linkmode=external -extldflags "-Wl,-z,muldefs -static" -v
endif

ifeq ($(VERBOSE),true)
	build_args += -v
endif

BUILD_TARGETS := build install

all: build install

build: BUILD_ARGS := -o $(BUILDDIR)

$(BUILD_TARGETS): go.sum $(BUILDDIR)/
	go $@ -mod=readonly $(BUILD_ARGS) ./...

$(BUILDDIR)/:
	mkdir -p $(BUILDDIR)/

build-docker:
	$(DOCKER) build --tag babylonchain/staking-indexer -f Dockerfile \
		$(shell git rev-parse --show-toplevel)

.PHONY: build build-docker

test:
	go test ./...

test-e2e:
	cd $(TOOLS_DIR); go install -trimpath $(BABYLON_PKG)
	go test -mod=readonly -timeout=25m -v $(PACKAGES_E2E) -count=1 --tags=e2e
