package dpos

import (
	"math/big"
	"sort"

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
	sort.Sort(types.SortCandidateContexts(candidates))
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

func (api *API) GetSortableAddresses(number *rpc.BlockNumber) (types.SortableAddresses, error) {
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
	sa, err := dposContext.GetSortableAddresses()
	if err != nil {
		return nil, err
	}
	return sa, nil
}

// GetMintCnt retrieves the mint cnt of the validator at specified block
func (api *API) GetMintCnt(addr common.Address, number *rpc.BlockNumber) (int64, error) {
	var header *types.Header
	cnt := int64(0)
	if number == nil || *number == rpc.LatestBlockNumber {
		header = api.chain.CurrentHeader()
	} else {
		header = api.chain.GetHeaderByNumber(uint64(number.Int64()))
	}
	if header == nil {
		return cnt, errUnknownBlock
	}

	dposContext, err := types.NewDposContextFromProto(api.dpos.db, header.DposContext)
	if err != nil {
		return cnt, err
	}
	cnt = dposContext.GetMintCnt(addr)
	return cnt, nil
}

// GetMintCnt retrieves all the mint cnt of all validators at specified block
func (api *API) GetMintCnts(number *rpc.BlockNumber) ([]types.MintCntAddress, error) {
	var header *types.Header
	ma := make([]types.MintCntAddress, 0)
	if number == nil || *number == rpc.LatestBlockNumber {
		header = api.chain.CurrentHeader()
	} else {
		header = api.chain.GetHeaderByNumber(uint64(number.Int64()))
	}
	if header == nil {
		return ma, errUnknownBlock
	}

	dposContext, err := types.NewDposContextFromProto(api.dpos.db, header.DposContext)
	if err != nil {
		return ma, err
	}
	return dposContext.GetMintCnts()
}

// GetVote retrieves delegator to candidate at specified block
func (api *API) GetVote(addr common.Address, number *rpc.BlockNumber) (map[string]string, error) {
	var header *types.Header
	vote := make(map[string]string)
	if number == nil || *number == rpc.LatestBlockNumber {
		header = api.chain.CurrentHeader()
	} else {
		header = api.chain.GetHeaderByNumber(uint64(number.Int64()))
	}
	if header == nil {
		return vote, errUnknownBlock
	}

	dposContext, err := types.NewDposContextFromProto(api.dpos.db, header.DposContext)
	if err != nil {
		return vote, err
	}
	return dposContext.GetVote(addr)
}

// GetVotes retrieves all delegator to their candidate at specified block
func (api *API) GetVotes(number *rpc.BlockNumber) (map[string]string, error) {
	var header *types.Header
	votes := make(map[string]string)

	if number == nil || *number == rpc.LatestBlockNumber {
		header = api.chain.CurrentHeader()
	} else {
		header = api.chain.GetHeaderByNumber(uint64(number.Int64()))
	}
	if header == nil {
		return votes, errUnknownBlock
	}

	dposContext, err := types.NewDposContextFromProto(api.dpos.db, header.DposContext)
	if err != nil {
		return votes, err
	}
	return dposContext.GetVotes()
}

// GetCFDs retrieves all candidates from their delegates at specified block
func (api *API) GetCFDs(number *rpc.BlockNumber) (map[string]map[string]*big.Int, error) {
	var header *types.Header
	cfd := make(map[string]map[string]*big.Int)

	if number == nil || *number == rpc.LatestBlockNumber {
		header = api.chain.CurrentHeader()
	} else {
		header = api.chain.GetHeaderByNumber(uint64(number.Int64()))
	}
	if header == nil {
		return cfd, errUnknownBlock
	}

	dposContext, err := types.NewDposContextFromProto(api.dpos.db, header.DposContext)
	if err != nil {
		return cfd, err
	}
	return dposContext.GetCFDs()
}
