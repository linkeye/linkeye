package dpos

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/linkeye/linkeye/common"
	"github.com/linkeye/linkeye/core/state"
	"github.com/linkeye/linkeye/core/types"
	"github.com/linkeye/linkeye/log"
	"github.com/linkeye/linkeye/rlp"
	"github.com/linkeye/linkeye/trie"
)

type EpochContext struct {
	TimeStamp   int64
	DposContext *types.DposContext
	statedb     *state.StateDB

	Number  uint64                      `json:"number"`  // Block number
	Hash    common.Hash                 `json:"hash"`    // Block hash
	Signers map[common.Address]struct{} `json:"signers"` // Set of authorized signers at this moment
	Recents map[uint64]common.Address   `json:"recents"` // Set of recent signers for spam protections
}

// newEpochContext creates a new EpochContext with the specified startup parameters. This
// method does not initialize the set of recent signers, so only ever use if for
// the genesis block.
func newEpochContext(number uint64, hash common.Hash, dposContext *types.DposContext, state *state.StateDB) (epoch *EpochContext, err error) {
	epoch = &EpochContext{
		DposContext: dposContext,
		statedb:     state,
		Number:      number,
		Hash:        hash,
		Signers:     make(map[common.Address]struct{}),
		Recents:     make(map[uint64]common.Address),
	}

	// get signer list
	validators, err := dposContext.GetValidators()
	if err != nil {
		return nil, err
	}

	for _, signer := range validators {
		epoch.Signers[signer] = struct{}{}
	}

	return epoch, nil
}

// countVotes
func (ec *EpochContext) countVotes() (votes map[common.Address]*big.Int, err error) {
	votes = map[common.Address]*big.Int{}
	delegateTrie := ec.DposContext.DelegateTrie()
	candidateTrie := ec.DposContext.CandidateTrie()
	statedb := ec.statedb

	iterCandidate := trie.NewIterator(candidateTrie.NodeIterator(nil))
	existCandidate := iterCandidate.Next()
	if !existCandidate {
		return votes, errors.New("no candidates")
	}
	for existCandidate {
		var cc types.CandidateContext
		candidate := iterCandidate.Value
		rlp.DecodeBytes(candidate, &cc)
		candidateAddr := common.BytesToAddress(cc.Addr.Bytes())
		delegateIterator := trie.NewIterator(delegateTrie.PrefixIterator(candidate))
		existDelegator := delegateIterator.Next()
		if !existDelegator {
			votes[candidateAddr] = new(big.Int)
			existCandidate = iterCandidate.Next()
			continue
		}
		for existDelegator {
			delegator := delegateIterator.Value
			score, ok := votes[candidateAddr]
			if !ok {
				score = new(big.Int)
			}
			delegatorAddr := common.BytesToAddress(delegator)
			weight := statedb.GetBalance(delegatorAddr)
			score.Add(score, weight)
			votes[candidateAddr] = score
			existDelegator = delegateIterator.Next()
		}
		existCandidate = iterCandidate.Next()
	}
	return votes, nil
}

func (ec *EpochContext) kickoutValidator() error {
	validators, err := ec.DposContext.GetValidators()
	if err != nil {
		return fmt.Errorf("failed to get validator: %s", err)
	}
	if len(validators) == 0 {
		return errors.New("no validator could be kickout")
	}

	needKickoutValidators := types.SortableAddresses{}
	for _, validator := range validators {

		key := validator.Bytes()
		cnt := int64(0)
		if cntBytes := ec.DposContext.MintCntTrie().Get(key); cntBytes != nil {
			cnt = int64(binary.BigEndian.Uint64(cntBytes))
		}
		if cnt < int64(epochLength)/maxValidatorSize/2 {
			// not active validators need kickout
			needKickoutValidators = append(needKickoutValidators, &types.SortableAddress{validator, big.NewInt(cnt)})
		}
	}
	// clear mintcnt trie
	mintCntTrie, _ := types.NewMintCntTrie(common.Hash{}, ec.DposContext.DB())
	ec.DposContext.SetMintCnt(mintCntTrie)

	// no validators need kickout
	needKickoutValidatorCnt := len(needKickoutValidators)
	if needKickoutValidatorCnt <= 0 {
		return nil
	}
	sort.Sort(sort.Reverse(needKickoutValidators))

	candidateCount := 0
	iter := trie.NewIterator(ec.DposContext.CandidateTrie().NodeIterator(nil))
	for iter.Next() {
		candidateCount++
		if candidateCount >= needKickoutValidatorCnt+safeSize {
			break
		}
	}

	for i, validator := range needKickoutValidators {
		// ensure candidate count greater than or equal to safeSize
		if candidateCount <= safeSize {
			log.Info("No more candidate can be kickout", "candidateCount", candidateCount, "needKickoutCount", len(needKickoutValidators)-i)
			return nil
		}

		if err := ec.DposContext.KickoutCandidate(validator.Address); err != nil {
			return err
		}
		// if kickout success, candidateCount minus 1
		candidateCount--
		log.Info("Kickout candidate", "candidate", validator.Address.String(), "mintCnt", validator.Weight.String())
	}
	return nil
}

// update counts in MintCntTrie for the miner of newBlock
func (ec *EpochContext) updateMintCnt(validator common.Address) {
	currentMintCntTrie := ec.DposContext.MintCntTrie()

	cnt := int64(1)

	cntBytes := currentMintCntTrie.Get(validator.Bytes())

	// not the first time to mint
	if cntBytes != nil {
		cnt = int64(binary.BigEndian.Uint64(cntBytes)) + 1
	}

	newCntBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(newCntBytes, uint64(cnt))
	ec.DposContext.MintCntTrie().TryUpdate(validator.Bytes(), newCntBytes)
}

func (ec *EpochContext) tryElect(genesis, parent *types.Header) error {

	// if prevEpoch is not genesis, kickout not active candidate
	/*if parent.Number.Uint64() != 0 {
		if err := ec.kickoutValidator(); err != nil {
			return err
		}
	}*/

	votes, err := ec.countVotes()
	if err != nil {
		return err
	}
	candidates := types.SortableAddresses{}
	for candidate, cnt := range votes {
		candidates = append(candidates, &types.SortableAddress{candidate, cnt})
	}
	if len(candidates) < safeSize {
		return errors.New("too few candidates")
	}
	sort.Sort(candidates)
	if len(candidates) > maxValidatorSize {
		candidates = candidates[:maxValidatorSize]
	}

	sortedValidators := make([]common.Address, 0)
	for _, candidate := range candidates {
		sortedValidators = append(sortedValidators, candidate.Address)
	}

	epochTrie, _ := types.NewEpochTrie(common.Hash{}, ec.DposContext.DB())
	ec.DposContext.SetEpoch(epochTrie)
	ec.DposContext.SetValidators(sortedValidators)
	ec.DposContext.SetSortableAddresses(candidates)

	return nil
}

// signers retrieves the list of authorized signers in ascending order.
func (s *EpochContext) signers() []common.Address {
	signers := make([]common.Address, 0, len(s.Signers))
	for signer := range s.Signers {
		signers = append(signers, signer)
	}
	for i := 0; i < len(signers); i++ {
		for j := i + 1; j < len(signers); j++ {
			if bytes.Compare(signers[i][:], signers[j][:]) > 0 {
				signers[i], signers[j] = signers[j], signers[i]
			}
		}
	}
	return signers
}

// inturn returns if a signer at a given block height is in-turn or not.
func (s *EpochContext) inturn(number uint64, signer common.Address) bool {
	signers, offset := s.signers(), 0
	for offset < len(signers) && signers[offset] != signer {
		offset++
	}
	return (number % uint64(len(signers))) == uint64(offset)
}

// inturn returns if a signer at a given block height is in-turn or not.
func (s *EpochContext) signerIndex(signer common.Address) uint64 {
	signers, offset := s.signers(), 0
	for offset < len(signers) && signers[offset] != signer {
		offset++
	}
	return uint64(offset)
}
