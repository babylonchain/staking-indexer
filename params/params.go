package params

import (
	"github.com/babylonchain/networks/parameters/parser"
)

type ParamsRetriever interface {
	VersionedParams() *parser.ParsedGlobalParams
}

type GlobalParamsRetriever struct {
	paramsVersions *parser.ParsedGlobalParams
}

func NewGlobalParamsRetriever(filePath string) (*GlobalParamsRetriever, error) {
	parsedGlobalParams, err := parser.NewParsedGlobalParamsFromFile(filePath)
	if err != nil {
		return nil, err
	}

	return &GlobalParamsRetriever{paramsVersions: parsedGlobalParams}, nil
}

func (lp *GlobalParamsRetriever) VersionedParams() *parser.ParsedGlobalParams {
	return lp.paramsVersions
}
