package dpos

import (
	"github.com/linkeye/linkeye/common"
	"github.com/linkeye/linkeye/consensus"
	"github.com/linkeye/linkeye/core/types"
	"github.com/linkeye/linkeye/rpc"
)

// API is a user facing RPC API to allow controlling the delegate and voting
// mechanisms of the delegated-proof-of-stake
type API struct {
	chain consensus.ChainReader
	dpos  *DPOS
}

// GetValidators retrieves the list of the validators at specified block
func (api *API) GetValidators(number *rpc.BlockNumber) ([]common.Address, error) {
	var header *types.Header
	if number == nil || *number == rpc.LatestBlockNumber {
		header = api.chain.CurrentHeader()
	} else {
		header = api.chain.GetHeaderByNumber(uint64(number.Int64()))
	}
	if header == nil {
		return nil, errUnknownBlock
	}

	dposContext, err := types.NewDposContextFromProto(api.dpos.db, header.DposContext)
	if err != nil {
		return nil, err
	}
	validators, err := dposContext.GetValidators()
	if err != nil {
		return nil, err
	}
	return validators, nil
}

// GetCandidates retrieves the list of the candidates at specified block
func (api *API) GetCandidates(number *rpc.BlockNumber) ([]types.CandidateContext, error) {
	var header *types.Header
	if number == nil || *number == rpc.LatestBlockNumber {
		header = api.chain.CurrentHeader()
	} else {
		header = api.chain.GetHeaderByNumber(uint64(number.Int64()))
	}
	if header == nil {
		return nil, errUnknownBlock
	}

	dposContext, err := types.NewDposContextFromProto(api.dpos.db, header.DposContext)
	if err != nil {
		return nil, err
	}
	candidates, err := dposContext.GetCandidates()
	if err != nil {
		return nil, err
	}
	return candidates, nil
}

// GetCandidates retrieves the list of the candidates at specified block
func (api *API) GetCandidate(addr common.Address, number *rpc.BlockNumber) (types.CandidateContext, error) {
	var header *types.Header
	var cc types.CandidateContext
	if number == nil || *number == rpc.LatestBlockNumber {
		header = api.chain.CurrentHeader()
	} else {
		header = api.chain.GetHeaderByNumber(uint64(number.Int64()))
	}
	if header == nil {
		return cc, errUnknownBlock
	}

	dposContext, err := types.NewDposContextFromProto(api.dpos.db, header.DposContext)
	if err != nil {
		return cc, err
	}
	candidatecontext, err := dposContext.GetCandidateContext(addr)
	if err != nil {
		return cc, err
	}
	return candidatecontext, nil
}
