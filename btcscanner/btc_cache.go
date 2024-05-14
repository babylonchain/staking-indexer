package btcscanner

import (
	"sort"
	"sync"

	"github.com/babylonchain/staking-indexer/types"
)

const defaultMaxEntries = 100

type BTCCache struct {
	blocks     []*types.IndexedBlock
	maxEntries uint64

	sync.RWMutex
}

func NewBTCCache(maxEntries uint64) (*BTCCache, error) {
	// if maxEntries is 0, it means that the cache is disabled
	if maxEntries == 0 {
		return nil, ErrInvalidMaxEntries
	}

	return &BTCCache{
		blocks:     make([]*types.IndexedBlock, 0, maxEntries),
		maxEntries: maxEntries,
	}, nil
}

// Init initializes the cache with the given blocks. Input blocks should be sorted by height. Thread-safe.
func (b *BTCCache) Init(ibs []*types.IndexedBlock) error {
	b.Lock()
	defer b.Unlock()

	if len(ibs) > int(b.maxEntries) {
		return ErrTooManyEntries
	}

	// check if the blocks are sorted by height
	if sortedByHeight := sort.SliceIsSorted(ibs, func(i, j int) bool {
		return ibs[i].Height < ibs[j].Height
	}); !sortedByHeight {
		return ErrUnsortedBlocks
	}

	for _, ib := range ibs {
		b.add(ib)
	}

	return nil
}

// Add adds a new block to the cache. Thread-safe.
func (b *BTCCache) Add(ib *types.IndexedBlock) {
	b.Lock()
	defer b.Unlock()

	b.add(ib)
}

// Thread-unsafe version of Add
func (b *BTCCache) add(ib *types.IndexedBlock) {
	if b.size() > b.maxEntries {
		panic(ErrTooManyEntries)
	}
	if b.size() == b.maxEntries {
		// dereference the 0-th block to ensure it will be garbage-collected
		// see https://stackoverflow.com/questions/55045402/memory-leak-in-golang-slice
		b.blocks[0] = nil
		b.blocks = b.blocks[1:]
	}

	b.blocks = append(b.blocks, ib)
}

func (b *BTCCache) First() *types.IndexedBlock {
	b.RLock()
	defer b.RUnlock()

	if b.size() == 0 {
		return nil
	}

	return b.blocks[0]
}

func (b *BTCCache) Tip() *types.IndexedBlock {
	b.RLock()
	defer b.RUnlock()

	if b.size() == 0 {
		return nil
	}

	return b.blocks[len(b.blocks)-1]
}

// RemoveAll deletes all the blocks in cache
func (b *BTCCache) RemoveAll() {
	b.Lock()
	defer b.Unlock()

	b.blocks = []*types.IndexedBlock{}
}

// Size returns the size of the cache. Thread-safe.
func (b *BTCCache) Size() uint64 {
	b.RLock()
	defer b.RUnlock()

	return b.size()
}

// thread-unsafe version of Size
func (b *BTCCache) size() uint64 {
	return uint64(len(b.blocks))
}

func (b *BTCCache) GetAllBlocks() []*types.IndexedBlock {
	b.RLock()
	defer b.RUnlock()

	l := len(b.blocks)
	res := make([]*types.IndexedBlock, l)
	copy(res, b.blocks)

	return res
}

// GetLastBlocks returns the last k blocks
func (b *BTCCache) GetLastBlocks(k int) []*types.IndexedBlock {
	b.RLock()
	defer b.RUnlock()

	var res []*types.IndexedBlock
	l := len(b.blocks)
	if l <= k {
		blocksCopy := make([]*types.IndexedBlock, l)
		copy(blocksCopy, b.blocks)
		res = blocksCopy
	} else {
		blocksCopy := make([]*types.IndexedBlock, k)
		copy(blocksCopy, b.blocks[l-k:])
		res = blocksCopy
	}

	return res
}

// TrimConfirmedBlocks keeps the last <=k blocks in the cache and returns the rest in the same order
// the returned blocks are considered confirmed
func (b *BTCCache) TrimConfirmedBlocks(k int) []*types.IndexedBlock {
	b.Lock()
	defer b.Unlock()

	l := len(b.blocks)
	if l <= k {
		return nil
	}

	res := make([]*types.IndexedBlock, l-k)
	copy(res, b.blocks)
	b.blocks = b.blocks[l-k:]

	return res
}
