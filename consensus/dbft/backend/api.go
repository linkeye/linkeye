package backend

import (
	"math/big"
	"sort"

	"github.com/linkeye/linkeye/common"
	"github.com/linkeye/linkeye/consensus"
	"github.com/linkeye/linkeye/core/types"
	"github.com/linkeye/linkeye/rpc"
)

// API is a user facing RPC API to dump BFT state
type API struct {
	chain consensus.ChainReader
	bft   *backend
}

// GetSnapshot retrieves the state snapshot at a given block.
func (api *API) GetSnapshot(number *rpc.BlockNumber) (*Snapshot, error) {
	// Retrieve the requested block number (or current if none requested)
	var header *types.Header
	if number == nil || *number == rpc.LatestBlockNumber {
		header = api.chain.CurrentHeader()
	} else {
		header = api.chain.GetHeaderByNumber(uint64(number.Int64()))
	}
	// Ensure we have an actually valid block and return its snapshot
	if header == nil {
		return nil, errUnknownBlock
	}
	return api.bft.snapshot(api.chain, header.Number.Uint64(), header.Hash(), nil)
}

// GetSnapshotAtHash retrieves the state snapshot at a given block.
func (api *API) GetSnapshotAtHash(hash common.Hash) (*Snapshot, error) {
	header := api.chain.GetHeaderByHash(hash)
	if header == nil {
		return nil, errUnknownBlock
	}
	return api.bft.snapshot(api.chain, header.Number.Uint64(), header.Hash(), nil)
}

// GetValidators retrieves the list of authorized validators at the specified block.
func (api *API) GetValidators(number *rpc.BlockNumber) ([]common.Address, error) {
	// Retrieve the requested block number (or current if none requested)
	var header *types.Header
	if number == nil || *number == rpc.LatestBlockNumber {
		header = api.chain.CurrentHeader()
	} else {
		header = api.chain.GetHeaderByNumber(uint64(number.Int64()))
	}
	// Ensure we have an actually valid block and return the validators from its snapshot
	if header == nil {
		return nil, errUnknownBlock
	}
	snap, err := api.bft.snapshot(api.chain, header.Number.Uint64(), header.Hash(), nil)
	if err != nil {
		return nil, err
	}
	return snap.validators(), nil
}

// GetValidatorsAtHash retrieves the state snapshot at a given block.
func (api *API) GetValidatorsAtHash(hash common.Hash) ([]common.Address, error) {
	header := api.chain.GetHeaderByHash(hash)
	if header == nil {
		return nil, errUnknownBlock
	}
	snap, err := api.bft.snapshot(api.chain, header.Number.Uint64(), header.Hash(), nil)
	if err != nil {
		return nil, err
	}
	return snap.validators(), nil
}

// Candidates returns the current candidates the node tries to uphold and vote on.
func (api *API) Candidates() map[common.Address]bool {
	api.bft.candidatesLock.RLock()
	defer api.bft.candidatesLock.RUnlock()

	proposals := make(map[common.Address]bool)
	for address, auth := range api.bft.candidates {
		proposals[address] = auth
	}
	return proposals
}

// Propose injects a new authorization candidate that the validator will attempt to
// push through.
func (api *API) Propose(address common.Address, auth bool) {
	api.bft.candidatesLock.Lock()
	defer api.bft.candidatesLock.Unlock()

	api.bft.candidates[address] = auth
}

// Discard drops a currently running candidate, stopping the validator from casting
// further votes (either for or against).
func (api *API) Discard(address common.Address) {
	api.bft.candidatesLock.Lock()
	defer api.bft.candidatesLock.Unlock()

	delete(api.bft.candidates, address)
}

//copy from dpos.api
// GetDposValidators retrieves the list of the validators at specified block
func (api *API) GetDposValidators(number *rpc.BlockNumber) ([]common.Address, error) {
	var header *types.Header
	if number == nil || *number == rpc.LatestBlockNumber {
		header = api.chain.CurrentHeader()
	} else {
		header = api.chain.GetHeaderByNumber(uint64(number.Int64()))
	}
	if header == nil {
		return nil, errUnknownBlock
	}

	dposContext, err := types.NewDposContextFromProto(api.bft.db, header.DposContext)
	if err != nil {
		return nil, err
	}
	validators, err := dposContext.GetValidators()
 	if err != nil {
		return nil, err
	}
	return validators, nil
}

// GetCandidates retrieves maxcnt result of the list of the candidates at specified block
func (api *API) GetDposCandidates(maxcnt, number *rpc.BlockNumber) ([]types.CandidateContext, error) {
	var header *types.Header
	if number == nil || *number == rpc.LatestBlockNumber {
		header = api.chain.CurrentHeader()
	} else {
		header = api.chain.GetHeaderByNumber(uint64(number.Int64()))
	}
	if header == nil {
		return nil, errUnknownBlock
	}

	dposContext, err := types.NewDposContextFromProto(api.bft.DB(), header.DposContext)
	if err != nil {
		return nil, err
	}
	candidates, err := dposContext.GetCandidates()
	if err != nil {
		return nil, err
	}
	sort.Sort(types.SortCandidateContexts(candidates))
	if maxcnt == nil {
		return candidates, nil
	}
	index := int(maxcnt.Int64())
	if int(maxcnt.Int64()) > len(candidates) {
		index = len(candidates)
	}
	return candidates[:index], nil
}

// GetDposCandidates retrieves the list of the candidates at specified block
func (api *API) GetDposCandidate(addr common.Address, number *rpc.BlockNumber) (types.CandidateContext, error) {
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

	dposContext, err := types.NewDposContextFromProto(api.bft.DB(), header.DposContext)
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

	dposContext, err := types.NewDposContextFromProto(api.bft.DB(), header.DposContext)
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

	dposContext, err := types.NewDposContextFromProto(api.bft.DB(), header.DposContext)
	if err != nil {
		return cnt, err
	}
	cnt = dposContext.GetMintCnt(addr)
	return cnt, nil
}

// GetMintCnt retrieves maxcnt result of the mint cnt of all validators at specified block
func (api *API) GetMintCnts(maxcnt, number *rpc.BlockNumber) ([]types.MintCntAddress, error) {
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

	dposContext, err := types.NewDposContextFromProto(api.bft.DB(), header.DposContext)
	if err != nil {
		return ma, err
	}
	ma, err = dposContext.GetMintCnts()
	sort.Sort(types.SortableMintCntAddresses(ma))
	if maxcnt == nil {
		return ma, err
	}
	index := int(maxcnt.Int64())
	if int(maxcnt.Int64()) > len(ma) {
		index = len(ma)
	}
	return ma[:index], nil
}

// GetVote retrieves delegator to candidate at specified block
func (api *API) GetVote(addr common.Address, number *rpc.BlockNumber) (map[string]types.VoteContext, error) {
	var header *types.Header
	vote := make(map[string]types.VoteContext)
	if number == nil || *number == rpc.LatestBlockNumber {
		header = api.chain.CurrentHeader()
	} else {
		header = api.chain.GetHeaderByNumber(uint64(number.Int64()))
	}
	if header == nil {
		return vote, errUnknownBlock
	}

	dposContext, err := types.NewDposContextFromProto(api.bft.DB(), header.DposContext)
	if err != nil {
		return vote, err
	}
	return dposContext.GetVote(addr)
}

// GetVotes retrieves all delegator to their candidate at specified block
func (api *API) GetVotes(number *rpc.BlockNumber) (map[string]types.VoteContext, error) {
	var header *types.Header
	votes := make(map[string]types.VoteContext)

	if number == nil || *number == rpc.LatestBlockNumber {
		header = api.chain.CurrentHeader()
	} else {
		header = api.chain.GetHeaderByNumber(uint64(number.Int64()))
	}
	if header == nil {
		return votes, errUnknownBlock
	}

	dposContext, err := types.NewDposContextFromProto(api.bft.DB(), header.DposContext)
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

	dposContext, err := types.NewDposContextFromProto(api.bft.DB(), header.DposContext)
	if err != nil {
		return cfd, err
	}
	return dposContext.GetCFDs()
}
