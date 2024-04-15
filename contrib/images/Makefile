DOCKER = $(shell which docker)

.PHONY: staking-indexer staking-indexer-rmi

staking-indexer:
	$(DOCKER) build --tag babylonchain/staking-indexer -f staking-indexer/Dockerfile \
				$(shell git rev-parse --show-toplevel)

staking-indexer-rmi:
	$(DOCKER) rmi babylonchain/staking-indexer 2>/dev/null; true
