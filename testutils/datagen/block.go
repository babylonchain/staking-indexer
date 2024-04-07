package datagen

import (
	"math"
	"math/big"
	"math/rand"

	"github.com/babylonchain/babylon/testutil/datagen"
	bbntypes "github.com/babylonchain/babylon/types"
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"

	"github.com/babylonchain/staking-indexer/types"
)

func GenRandomBlock(r *rand.Rand, prevHash *chainhash.Hash) *wire.MsgBlock {
	coinbaseTx := GenRandomTx(r)
	msgTxs := []*wire.MsgTx{coinbaseTx}

	// calculate correct Merkle root
	merkleRoot := calcMerkleRoot(msgTxs)
	// don't apply any difficulty
	difficulty, _ := new(big.Int).SetString("7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 16)
	workBits := blockchain.BigToCompact(difficulty)

	// find a header that satisfies difficulty
	var header *wire.BlockHeader
	for {
		header = datagen.GenRandomBtcdHeader(r)
		header.MerkleRoot = merkleRoot
		header.Bits = workBits

		if prevHash == nil {
			header.PrevBlock = chainhash.DoubleHashH(datagen.GenRandomByteArray(r, 10))
		} else {
			header.PrevBlock = *prevHash
		}

		if err := bbntypes.ValidateBTCHeader(header, chaincfg.SimNetParams.PowLimit); err == nil {
			break
		}
	}

	block := &wire.MsgBlock{
		Header:       *header,
		Transactions: msgTxs,
	}
	return block
}

// GetRandomIndexedBlocks generates a random number of indexed blocks with a random root height
func GetRandomIndexedBlocks(r *rand.Rand, numBlocks uint64) []*types.IndexedBlock {
	var ibs []*types.IndexedBlock

	if numBlocks == 0 {
		return ibs
	}

	block := GenRandomBlock(r, nil)
	prevHeight := r.Int31n(math.MaxInt32 - int32(numBlocks))
	ib := types.NewIndexedBlockFromMsgBlock(prevHeight, block)
	prevHash := ib.Header.BlockHash()

	ibs = GetRandomIndexedBlocksFromHeight(r, numBlocks-1, prevHeight, prevHash)
	ibs = append([]*types.IndexedBlock{ib}, ibs...)
	return ibs
}

// GetRandomIndexedBlocksFromHeight generates a random number of indexed blocks with a given root height and root hash
func GetRandomIndexedBlocksFromHeight(r *rand.Rand, numBlocks uint64, rootHeight int32, rootHash chainhash.Hash) []*types.IndexedBlock {
	var (
		ibs        []*types.IndexedBlock
		prevHash   = rootHash
		prevHeight = rootHeight
	)

	for i := 0; i < int(numBlocks); i++ {
		block := GenRandomBlock(r, &prevHash)
		newIb := types.NewIndexedBlockFromMsgBlock(prevHeight+1, block)
		ibs = append(ibs, newIb)

		prevHeight = newIb.Height
		prevHash = newIb.Header.BlockHash()
	}

	return ibs
}

// calcMerkleRoot creates a merkle tree from the slice of transactions and
// returns the root of the tree.
// (taken from https://github.com/btcsuite/btcd/blob/master/blockchain/fullblocktests/generate.go)
func calcMerkleRoot(txns []*wire.MsgTx) chainhash.Hash {
	if len(txns) == 0 {
		return chainhash.Hash{}
	}

	utilTxns := make([]*btcutil.Tx, 0, len(txns))
	for _, tx := range txns {
		utilTxns = append(utilTxns, btcutil.NewTx(tx))
	}
	merkles := blockchain.BuildMerkleTreeStore(utilTxns, false)
	return *merkles[len(merkles)-1]
}
