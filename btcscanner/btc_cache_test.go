package btcscanner_test

import (
	"math/rand"
	"testing"

	bbndatagen "github.com/babylonchain/babylon/testutil/datagen"
	"github.com/stretchr/testify/require"

	"github.com/babylonchain/staking-indexer/btcscanner"
	"github.com/babylonchain/staking-indexer/testutils/datagen"
)

// FuzzBtcCache fuzzes the BtcCache type
func FuzzBtcCache(f *testing.F) {
	bbndatagen.AddRandomSeedsToFuzzer(f, 100)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		// Create a new cache
		maxEntries := bbndatagen.RandomInt(r, 1000) + 2 // make sure we have at least 2 entries

		// 1/10 times generate invalid maxEntries
		invalidMaxEntries := false
		if bbndatagen.OneInN(r, 10) {
			maxEntries = 0
			invalidMaxEntries = true
		}

		cache, err := btcscanner.NewBTCCache(maxEntries)
		if err != nil {
			if !invalidMaxEntries {
				t.Fatalf("NewBTCCache returned error %s", err)
			}
			require.ErrorIs(t, err, btcscanner.ErrInvalidMaxEntries)
			t.Skip("Skipping test with invalid maxEntries")
		}

		// Generate a random number of blocks
		numBlocks := bbndatagen.RandomIntOtherThan(r, 0, int(maxEntries)) // make sure we have at least 1 entry

		// 1/10 times generate invalid number of blocks
		invalidNumBlocks := false
		if bbndatagen.OneInN(r, 10) {
			numBlocks = maxEntries + 1
			invalidNumBlocks = true
		}

		ibs := datagen.GetRandomIndexedBlocks(r, numBlocks)

		// Add all indexed blocks to the cache
		err = cache.Init(ibs)
		if err != nil {
			if !invalidNumBlocks {
				t.Fatalf("Cache init returned error %v", err)
			}
			require.ErrorIs(t, err, btcscanner.ErrTooManyEntries)
			t.Skip("Skipping test with invalid numBlocks")
		}

		require.Equal(t, numBlocks, cache.Size())

		k := r.Intn(int(maxEntries)) + 1
		lastBlocks := cache.GetLastBlocks(k)
		l := cache.Size()
		cacheAllBlocks := cache.GetAllBlocks()
		if uint64(k) <= l {
			require.Equal(t, k, len(lastBlocks))
			for i, b := range cacheAllBlocks[int(l)-k:] {
				require.Equal(t, b, lastBlocks[i])
			}
		} else {
			require.Equal(t, int(l), len(lastBlocks))
			for i, b := range cacheAllBlocks {
				require.Equal(t, b, lastBlocks[i])
			}
		}

		// Add random blocks to the cache
		addCount := bbndatagen.RandomIntOtherThan(r, 0, 1000)
		prevCacheHeight := cache.Tip().Height
		cacheBlocksBeforeAddition := cache.GetAllBlocks()
		blocksToAdd := datagen.GetRandomIndexedBlocksFromHeight(r, addCount, cache.Tip().Height, cache.Tip().BlockHash())
		for _, ib := range blocksToAdd {
			cache.Add(ib)
		}
		require.Equal(t, prevCacheHeight+int32(addCount), cache.Tip().Height)
		require.Equal(t, blocksToAdd[addCount-1], cache.Tip())

		// ensure block heights in cache are in increasing order
		var heights []int32
		for _, ib := range cache.GetAllBlocks() {
			heights = append(heights, ib.Height)
		}
		require.IsIncreasing(t, heights)

		// we need to compare block slices before and after addition, there are 3 cases to consider:
		// if addCount+numBlocks>=maxEntries then
		// 1. addCount >= maxEntries
		// 2. addCount < maxEntries
		// else
		// 3. addCount+numBlocks < maxEntries
		// case 2 and 3 are the same, so below is simplified version
		cacheBlocksAfterAddition := cache.GetAllBlocks()
		if addCount >= maxEntries {
			// cache contains only new blocks, so compare all cache blocks with slice of blocksToAdd
			require.Equal(t, blocksToAdd[addCount-maxEntries:], cacheBlocksAfterAddition)
		} else {
			// cache contains both old and all the new blocks, so we need to compare
			// blocksToAdd with slice of cacheBlocksAfterAddition and also compare
			// slice of cacheBlocksBeforeAddition with slice of cacheBlocksAfterAddition

			// compare new blocks
			newBlocksInCache := cacheBlocksAfterAddition[len(cacheBlocksAfterAddition)-int(addCount):]
			require.Equal(t, blocksToAdd, newBlocksInCache)

			// comparing old blocks
			oldBlocksInCache := cacheBlocksAfterAddition[:len(cacheBlocksAfterAddition)-int(addCount)]
			require.Equal(t, cacheBlocksBeforeAddition[len(cacheBlocksBeforeAddition)-(len(cacheBlocksAfterAddition)-int(addCount)):], oldBlocksInCache)
		}
	})
}
