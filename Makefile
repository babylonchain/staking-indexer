BUILDDIR ?= $(CURDIR)/build

GO_BIN := ${GOPATH}/bin
ldflags := $(LDFLAGS)
build_tags := $(BUILD_TAGS)
build_args := $(BUILD_ARGS)

DOCKER := $(shell which docker)
CUR_DIR := $(shell pwd)
MOCKS_DIR=$(CUR_DIR)/testutils/mocks
MOCKGEN_REPO=github.com/golang/mock/mockgen
MOCKGEN_VERSION=v1.6.0
MOCKGEN_CMD=go run ${MOCKGEN_REPO}@${MOCKGEN_VERSION}

PACKAGES_E2E=$(shell go list ./... | grep '/itest')

ifeq ($(LINK_STATICALLY),true)
	ldflags += -linkmode=external -extldflags "-Wl,-z,muldefs -static" -v
endif

ifeq ($(VERBOSE),true)
	build_args += -v
endif

BUILD_TARGETS := build install
BUILD_FLAGS := --tags "$(build_tags)" --ldflags '$(ldflags)'

# Update changelog vars
ifneq (,$(SINCE_TAG))
       sinceTag := --since-tag $(SINCE_TAG)
endif
ifneq (,$(UPCOMING_TAG))
       upcomingTag := --future-release $(UPCOMING_TAG)
endif

all: build install

build: BUILD_ARGS := $(build_args) -o $(BUILDDIR)

$(BUILD_TARGETS): go.sum $(BUILDDIR)/
	go $@ -mod=readonly $(BUILD_FLAGS) $(BUILD_ARGS) ./...

$(BUILDDIR)/:
	mkdir -p $(BUILDDIR)/

build-docker:
	$(DOCKER) build --tag babylonchain/staking-indexer -f Dockerfile \
		$(shell git rev-parse --show-toplevel)

.PHONY: build build-docker

test:
	go test ./...

test-e2e:
	./itest/scripts/start_rabbitmq.sh;
	go test -mod=readonly -timeout=25m -v $(PACKAGES_E2E) -count=1 --tags=e2e

test-integration:
	rm -rf ./.testnets
	mkdir -p ./.testnets/rabbitmq_data
	mkdir -p ./.testnets/bitcoin
	mkdir -p ./.testnets/staking-indexer/data ./.testnets/staking-indexer/logs
	docker compose -f integration/docker-compose.yml down
	docker compose -f integration/docker-compose.yml up -d

mock-gen:
	mkdir -p $(MOCKS_DIR)
	$(MOCKGEN_CMD) -source=consumer/event_consumer.go -package mocks -destination $(MOCKS_DIR)/event_consumer.go
	$(MOCKGEN_CMD) -source=btcscanner/btc_scanner.go -package mocks -destination $(MOCKS_DIR)/btc_scanner.go
	$(MOCKGEN_CMD) -source=btcscanner/expected_btc_client.go -package mocks -destination $(MOCKS_DIR)/btc_client.go

.PHONY: mock-gen

proto-gen:
	@$(call print, "Compiling protos.")
	cd ./proto; ./gen_protos_docker.sh

.PHONY: proto-gen
