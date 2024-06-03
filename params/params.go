package params

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"

	"github.com/babylonchain/babylon/btcstaking"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"

	"github.com/babylonchain/staking-indexer/types"
)

func checkPositive(value uint64) error {
	if value == 0 {
		return fmt.Errorf("value must be positive")
	}
	return nil
}

func parseTimeLockValue(timelock uint64) (uint16, error) {
	if timelock > math.MaxUint16 {
		return 0, fmt.Errorf("timelock value %d is too large. Max: %d", timelock, math.MaxUint16)
	}

	if err := checkPositive(timelock); err != nil {
		return 0, fmt.Errorf("invalid timelock value: %w", err)
	}

	return uint16(timelock), nil
}

func parseConfirmationDepthValue(confirmationDepth uint64) (uint16, error) {
	if confirmationDepth > math.MaxUint16 {
		return 0, fmt.Errorf("timelock value %d is too large. Max: %d", confirmationDepth, math.MaxUint16)
	}

	if err := checkPositive(confirmationDepth); err != nil {
		return 0, fmt.Errorf("invalid confirmation depth value: %w", err)
	}

	return uint16(confirmationDepth), nil
}

// either staking cap and cap height should be positive
// if cap height is positive, then it should be greater
// than the activation height
func parseCap(stakingCap, capHeight, activationHeight uint64) (btcutil.Amount, uint64, error) {
	if stakingCap != 0 {
		if capHeight != 0 {
			return 0, 0, fmt.Errorf("only either of staking cap and cap height can be set")
		}

		parsedStakingCap, err := parseBtcValue(stakingCap)
		return parsedStakingCap, 0, err
	}

	if capHeight == 0 {
		return 0, 0, fmt.Errorf("either of staking cap and cap height must be set")
	}

	if capHeight <= activationHeight {
		return 0, 0, fmt.Errorf("the cap height %d should be greater than the activation height %d", capHeight, activationHeight)
	}

	return 0, capHeight, nil
}

func parseBtcValue(value uint64) (btcutil.Amount, error) {
	if value > math.MaxInt64 {
		return 0, fmt.Errorf("value %d is too large. Max: %d", value, math.MaxInt64)
	}

	if err := checkPositive(value); err != nil {
		return 0, fmt.Errorf("invalid btc value value: %w", err)
	}
	// retrun amount in satoshis
	return btcutil.Amount(value), nil
}

func parseUint32(value uint64) (uint32, error) {
	if value > math.MaxUint32 {
		return 0, fmt.Errorf("value %d is too large. Max: %d", value, math.MaxUint32)
	}

	if err := checkPositive(value); err != nil {
		return 0, fmt.Errorf("invalid value: %w", err)
	}

	return uint32(value), nil
}

// parseCovenantPubKeyFromHex parses public key string to btc public key
// the input should be 33 bytes
func parseCovenantPubKeyFromHex(pkStr string) (*btcec.PublicKey, error) {
	pkBytes, err := hex.DecodeString(pkStr)
	if err != nil {
		return nil, err
	}

	pk, err := btcec.ParsePubKey(pkBytes)
	if err != nil {
		return nil, err
	}

	return pk, nil
}

type ParamsRetriever interface {
	VersionedParams() *types.ParamsVersions
}

type GlobalParamsRetriever struct {
	paramsVersions *types.ParamsVersions
}

type GlobalParams struct {
	Versions []*VersionedGlobalParams `json:"versions"`
}

type VersionedGlobalParams struct {
	Version           uint64   `json:"version"`
	ActivationHeight  uint64   `json:"activation_height"`
	StakingCap        uint64   `json:"staking_cap"`
	CapHeight         uint64   `json:"cap_height"`
	Tag               string   `json:"tag"`
	CovenantPks       []string `json:"covenant_pks"`
	CovenantQuorum    uint64   `json:"covenant_quorum"`
	UnbondingTime     uint64   `json:"unbonding_time"`
	UnbondingFee      uint64   `json:"unbonding_fee"`
	MaxStakingAmount  uint64   `json:"max_staking_amount"`
	MinStakingAmount  uint64   `json:"min_staking_amount"`
	MaxStakingTime    uint64   `json:"max_staking_time"`
	MinStakingTime    uint64   `json:"min_staking_time"`
	ConfirmationDepth uint64   `json:"confirmation_depth"`
}

type ParsedGlobalParams struct {
	Versions []*ParsedVersionedGlobalParams
}

type ParsedVersionedGlobalParams struct {
	Version           uint64
	ActivationHeight  uint64
	StakingCap        btcutil.Amount
	CapHeight         uint64
	Tag               []byte
	CovenantPks       []*btcec.PublicKey
	CovenantQuorum    uint32
	UnbondingTime     uint16
	UnbondingFee      btcutil.Amount
	MaxStakingAmount  btcutil.Amount
	MinStakingAmount  btcutil.Amount
	MaxStakingTime    uint16
	MinStakingTime    uint16
	ConfirmationDepth uint16
}

func NewGlobalParamsRetriever(filePath string) (*GlobalParamsRetriever, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var globalParams GlobalParams
	err = json.Unmarshal(data, &globalParams)
	if err != nil {
		return nil, err
	}

	parsedGlobalParams, err := ParseGlobalParams(&globalParams)

	if err != nil {
		return nil, err
	}

	return &GlobalParamsRetriever{paramsVersions: parsedGlobalParams.ToGlobalParams()}, nil
}

func ParseGlobalParams(p *GlobalParams) (*ParsedGlobalParams, error) {
	if len(p.Versions) == 0 {
		return nil, fmt.Errorf("global params must have at least one version")
	}
	var parsedVersions []*ParsedVersionedGlobalParams

	for i, v := range p.Versions {
		vCopy := v
		cv, err := parseVersionedGlobalParams(vCopy)

		if err != nil {
			return nil, fmt.Errorf("invalid params with version %d: %w", vCopy.Version, err)
		}

		// Check latest version
		if len(parsedVersions) > 0 {
			pv := parsedVersions[len(parsedVersions)-1]

			lastStakingCap := FindLastStakingCap(p.Versions[:i])

			if cv.Version != pv.Version+1 {
				return nil, fmt.Errorf("invalid params with version %d. versions should be monotonically increasing by 1", cv.Version)
			}
			if cv.StakingCap != 0 && cv.StakingCap < btcutil.Amount(lastStakingCap) {
				return nil, fmt.Errorf("invalid params with version %d. staking cap cannot be decreased in later versions, last non-zero staking cap: %d, got: %d",
					cv.Version, lastStakingCap, cv.StakingCap)
			}
			if cv.ActivationHeight < pv.ActivationHeight {
				return nil, fmt.Errorf("invalid params with version %d. activation height cannot be overlapping between earlier and later versions", cv.Version)
			}
			if cv.ActivationHeight <= pv.CapHeight {
				return nil, fmt.Errorf("invalid params with version %d. activation height must be greater than the cap height of the previous version", cv.Version)
			}
		}

		parsedVersions = append(parsedVersions, cv)
	}

	return &ParsedGlobalParams{
		Versions: parsedVersions,
	}, nil
}

// FindLastStakingCap finds the last staking cap that is not zero
// it returns zero if not non-zero value is found
func FindLastStakingCap(prevVersions []*VersionedGlobalParams) uint64 {
	numPrevVersions := len(prevVersions)
	if len(prevVersions) == 0 {
		return 0
	}

	for i := numPrevVersions - 1; i >= 0; i-- {
		if prevVersions[i].StakingCap > 0 {
			return prevVersions[i].StakingCap
		}
	}

	return 0
}

func (lp *GlobalParamsRetriever) VersionedParams() *types.ParamsVersions {
	return lp.paramsVersions
}

func parseVersionedGlobalParams(p *VersionedGlobalParams) (*ParsedVersionedGlobalParams, error) {
	tag, err := hex.DecodeString(p.Tag)

	if err != nil {
		return nil, fmt.Errorf("invalid tag: %w", err)
	}

	if len(tag) != btcstaking.MagicBytesLen {
		return nil, fmt.Errorf("invalid tag length, expected %d, got %d", btcstaking.MagicBytesLen, len(p.Tag))
	}

	if len(p.CovenantPks) == 0 {
		return nil, fmt.Errorf("empty covenant public keys")
	}
	if p.CovenantQuorum > uint64(len(p.CovenantPks)) {
		return nil, fmt.Errorf("covenant quorum cannot be more than the amount of covenants")
	}

	quroum, err := parseUint32(p.CovenantQuorum)
	if err != nil {
		return nil, fmt.Errorf("invalid covenant quorum: %w", err)
	}

	var covenantKeys []*btcec.PublicKey
	for _, covPk := range p.CovenantPks {
		pk, err := parseCovenantPubKeyFromHex(covPk)
		if err != nil {
			return nil, fmt.Errorf("invalid covenant public key %s: %w", covPk, err)
		}

		covenantKeys = append(covenantKeys, pk)
	}

	maxStakingAmount, err := parseBtcValue(p.MaxStakingAmount)

	if err != nil {
		return nil, fmt.Errorf("invalid max_staking_amount: %w", err)
	}

	minStakingAmount, err := parseBtcValue(p.MinStakingAmount)

	if err != nil {
		return nil, fmt.Errorf("invalid min_staking_amount: %w", err)
	}

	if maxStakingAmount <= minStakingAmount {
		return nil, fmt.Errorf("max-staking-amount must be larger than min-staking-amount")
	}

	ubTime, err := parseTimeLockValue(p.UnbondingTime)
	if err != nil {
		return nil, fmt.Errorf("invalid unbonding_time: %w", err)
	}

	ubFee, err := parseBtcValue(p.UnbondingFee)
	if err != nil {
		return nil, fmt.Errorf("invalid unbonding_fee: %w", err)
	}

	maxStakingTime, err := parseTimeLockValue(p.MaxStakingTime)
	if err != nil {
		return nil, fmt.Errorf("invalid max_staking_time: %w", err)
	}

	minStakingTime, err := parseTimeLockValue(p.MinStakingTime)
	if err != nil {
		return nil, fmt.Errorf("invalid min_staking_time: %w", err)
	}

	// NOTE: Allow config when max-staking-time is equal to min-staking-time, as then
	// we can configure a fixed staking time.
	if maxStakingTime < minStakingTime {
		return nil, fmt.Errorf("max-staking-time must be larger or equalt min-staking-time")
	}

	confirmationDepth, err := parseConfirmationDepthValue(p.ConfirmationDepth)
	if err != nil {
		return nil, fmt.Errorf("invalid confirmation_depth: %w", err)
	}

	stakingCap, capHeight, err := parseCap(p.StakingCap, p.CapHeight, p.ActivationHeight)
	if err != nil {
		return nil, fmt.Errorf("invalid cap: %w", err)
	}

	return &ParsedVersionedGlobalParams{
		Version:           p.Version,
		ActivationHeight:  p.ActivationHeight,
		StakingCap:        stakingCap,
		CapHeight:         capHeight,
		Tag:               tag,
		CovenantPks:       covenantKeys,
		CovenantQuorum:    quroum,
		UnbondingTime:     ubTime,
		UnbondingFee:      ubFee,
		MaxStakingAmount:  maxStakingAmount,
		MinStakingAmount:  minStakingAmount,
		MaxStakingTime:    maxStakingTime,
		MinStakingTime:    minStakingTime,
		ConfirmationDepth: confirmationDepth,
	}, nil
}

func (g *ParsedGlobalParams) ToGlobalParams() *types.ParamsVersions {
	versionedParams := make([]*types.GlobalParams, len(g.Versions))
	for i, p := range g.Versions {
		globalParams := &types.GlobalParams{
			Version:           uint16(p.Version),
			ActivationHeight:  p.ActivationHeight,
			StakingCap:        p.StakingCap,
			CapHeight:         p.CapHeight,
			Tag:               p.Tag,
			CovenantPks:       p.CovenantPks,
			CovenantQuorum:    p.CovenantQuorum,
			UnbondingTime:     p.UnbondingTime,
			UnbondingFee:      p.UnbondingFee,
			MaxStakingAmount:  p.MaxStakingAmount,
			MinStakingAmount:  p.MinStakingAmount,
			MaxStakingTime:    p.MaxStakingTime,
			MinStakingTime:    p.MinStakingTime,
			ConfirmationDepth: p.ConfirmationDepth,
		}

		versionedParams[i] = globalParams
	}

	return &types.ParamsVersions{ParamsVersions: versionedParams}
}
