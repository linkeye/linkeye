package backend

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/linkeye/linkeye/common"
	bft "github.com/linkeye/linkeye/consensus/dbft"
	"github.com/linkeye/linkeye/consensus/dbft/validator"
	"github.com/linkeye/linkeye/core/state"
	"github.com/linkeye/linkeye/core/types"
	"github.com/linkeye/linkeye/letdb"
	"github.com/linkeye/linkeye/log"
	"github.com/linkeye/linkeye/rlp"
	"github.com/linkeye/linkeye/trie"
)

const (
	dbKeySnapshotPrefix = "dbft-snapshot"
)

// Vote represents a single vote that an authorized validator made to modify the
// list of authorizations.
type Vote struct {
	Validator common.Address `json:"validator"` // Authorized validator that cast this vote
	Block     uint64         `json:"block"`     // Block number the vote was cast in (expire old votes)
	Address   common.Address `json:"address"`   // Account being voted on to change its authorization
	Authorize bool           `json:"authorize"` // Whether to authorize or deauthorize the voted account
}

// Tally is a simple vote tally to keep the current score of votes. Votes that
// go against the proposal aren't counted since it's equivalent to not voting.
type Tally struct {
	Authorize bool `json:"authorize"` // Whether the vote it about authorizing or kicking someone
	Votes     int  `json:"votes"`     // Number of votes until now wanting to pass the proposal
}

// Snapshot is the state of the authorization voting at a given point in time.
type Snapshot struct {
	Epoch uint64 // The number of blocks after which to checkpoint and reset the pending votes

	Number uint64                   // Block number where the snapshot was created
	Hash   common.Hash              // Block hash where the snapshot was created
	Votes  []*Vote                  // List of votes cast in chronological order
	Tally  map[common.Address]Tally // Current vote tally to avoid recalculating
	ValSet bft.ValidatorSet         // Set of authorized validators at this moment

	DposContext *types.DposContext
	StateDB     *state.StateDB
	//Signers     map[common.Address]struct{} // Set of authorized signers at this moment
	//Recents     map[uint64]common.Address   // Set of recent signers for spam protections
}

// newSnapshot create a new snapshot with the specified startup parameters. This
// method does not initialize the set of recent validators, so only ever use if for
// the genesis block.
func newSnapshot(epoch uint64, number uint64, hash common.Hash, valSet bft.ValidatorSet, dposContext *types.DposContext, state *state.StateDB) *Snapshot {
	snap := &Snapshot{
		Epoch:  epoch,
		Number: number,
		Hash:   hash,
		ValSet: valSet,
		Tally:  make(map[common.Address]Tally),

		DposContext: dposContext,
		StateDB:     state,
		//Signers:     make(map[common.Address]struct{}),
		//Recents:     make(map[uint64]common.Address),
	}

	// // get signer list
	// validators, err := dposContext.GetValidators()
	// if err != nil {
	// 	return nil
	// }

	// for _, signer := range validators {
	// 	snap.Signers[signer] = struct{}{}
	// }

	return snap
}

// loadSnapshot loads an existing snapshot from the database.
func loadSnapshot(epoch uint64, db letdb.Database, hash common.Hash) (*Snapshot, error) {
	blob, err := db.Get(append([]byte(dbKeySnapshotPrefix), hash[:]...))
	if err != nil {
		return nil, err
	}
	snap := new(Snapshot)
	if err := json.Unmarshal(blob, snap); err != nil {
		return nil, err
	}
	snap.Epoch = epoch

	return snap, nil
}

// store inserts the snapshot into the database.
func (s *Snapshot) store(db letdb.Database) error {
	blob, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return db.Put(append([]byte(dbKeySnapshotPrefix), s.Hash[:]...), blob)
}

// copy creates a deep copy of the snapshot, though not the individual votes.
func (s *Snapshot) copy() *Snapshot {
	cpy := &Snapshot{
		Epoch:  s.Epoch,
		Number: s.Number,
		Hash:   s.Hash,
		ValSet: s.ValSet.Copy(),
		Votes:  make([]*Vote, len(s.Votes)),
		Tally:  make(map[common.Address]Tally),

		// DposContext: s.DposContext.Copy(),
		// StateDB:     s.StateDB.Copy(),//FIXME
		// Signers:     make(map[common.Address]struct{}),
		// Recents:     make(map[uint64]common.Address),
	}

	for address, tally := range s.Tally {
		cpy.Tally[address] = tally
	}
	copy(cpy.Votes, s.Votes)

	// for addr, st := range s.Signers {
	// 	cpy.Signers[addr] = st
	// }

	// for i, addr := range s.Recents {
	// 	cpy.Recents[i] = addr
	// }

	return cpy
}

// checkVote return whether it's a valid vote
func (s *Snapshot) checkVote(address common.Address, authorize bool) bool {
	_, validator := s.ValSet.GetByAddress(address)
	return (validator != nil && !authorize) || (validator == nil && authorize)
}

// cast adds a new vote into the tally.
func (s *Snapshot) cast(address common.Address, authorize bool) bool {
	// Ensure the vote is meaningful
	if !s.checkVote(address, authorize) {
		return false
	}
	// Cast the vote into an existing or new tally
	if old, ok := s.Tally[address]; ok {
		old.Votes++
		s.Tally[address] = old
	} else {
		s.Tally[address] = Tally{Authorize: authorize, Votes: 1}
	}
	return true
}

// uncast removes a previously cast vote from the tally.
func (s *Snapshot) uncast(address common.Address, authorize bool) bool {
	// If there's no tally, it's a dangling vote, just drop
	tally, ok := s.Tally[address]
	if !ok {
		return false
	}
	// Ensure we only revert counted votes
	if tally.Authorize != authorize {
		return false
	}
	// Otherwise revert the vote
	if tally.Votes > 1 {
		tally.Votes--
		s.Tally[address] = tally
	} else {
		delete(s.Tally, address)
	}
	return true
}

// apply creates a new authorization snapshot by applying the given headers to
// the original one.
func (s *Snapshot) apply(headers []*types.Header) (*Snapshot, error) {
	// Allow passing in no headers for cleaner code
	if len(headers) == 0 {
		return s, nil
	}
	// Sanity check that the headers can be applied
	for i := 0; i < len(headers)-1; i++ {
		if headers[i+1].Number.Uint64() != headers[i].Number.Uint64()+1 {
			return nil, errInvalidVotingChain
		}
	}
	if headers[0].Number.Uint64() != s.Number+1 {
		return nil, errInvalidVotingChain
	}
	// Iterate through the headers and create a new snapshot
	snap := s.copy()

	for _, header := range headers {
		// Remove any votes on checkpoint blocks
		number := header.Number.Uint64()
		if number%s.Epoch == 0 {
			snap.Votes = nil
			snap.Tally = make(map[common.Address]Tally)
		}
		// Resolve the authorization key and check against validators
		validator, err := ecrecover(header, recentAddresses)
		if err != nil {
			return nil, err
		}
		if _, v := snap.ValSet.GetByAddress(validator); v == nil {
			log.Error("apply", "validator", validator, "valSet", snap.ValSet)
			return nil, errUnauthorized
		}

		// Header authorized, discard any previous votes from the validator
		for i, vote := range snap.Votes {
			if vote.Validator == validator && vote.Address == header.Coinbase {
				// Uncast the vote from the cached tally
				snap.uncast(vote.Address, vote.Authorize)

				// Uncast the vote from the chronological list
				snap.Votes = append(snap.Votes[:i], snap.Votes[i+1:]...)
				break // only one vote allowed
			}
		}
		// Tally up the new vote from the validator
		var authorize bool
		switch {
		case bytes.Compare(header.Nonce[:], nonceAuthVote) == 0:
			authorize = true
		case bytes.Compare(header.Nonce[:], nonceDropVote) == 0:
			authorize = false
		default:
			return nil, errInvalidVote
		}
		if snap.cast(header.Coinbase, authorize) {
			snap.Votes = append(snap.Votes, &Vote{
				Validator: validator,
				Block:     number,
				Address:   header.Coinbase,
				Authorize: authorize,
			})
		}
		// If the vote passed, update the list of validators
		if tally := snap.Tally[header.Coinbase]; tally.Votes > snap.ValSet.Size()/2 {
			if tally.Authorize {
				snap.ValSet.AddValidator(header.Coinbase)
			} else {
				snap.ValSet.RemoveValidator(header.Coinbase)

				// Discard any previous votes the deauthorized validator cast
				for i := 0; i < len(snap.Votes); i++ {
					if snap.Votes[i].Validator == header.Coinbase {
						// Uncast the vote from the cached tally
						snap.uncast(snap.Votes[i].Address, snap.Votes[i].Authorize)

						// Uncast the vote from the chronological list
						snap.Votes = append(snap.Votes[:i], snap.Votes[i+1:]...)

						i--
					}
				}
			}
			// Discard any previous votes around the just changed account
			for i := 0; i < len(snap.Votes); i++ {
				if snap.Votes[i].Address == header.Coinbase {
					snap.Votes = append(snap.Votes[:i], snap.Votes[i+1:]...)
					i--
				}
			}
			delete(snap.Tally, header.Coinbase)
		}
	}
	snap.Number += uint64(len(headers))
	snap.Hash = headers[len(headers)-1].Hash()

	return snap, nil
}

// validators retrieves the list of authorized validators in ascending order.
func (s *Snapshot) validators() []common.Address {
	validators := make([]common.Address, 0, s.ValSet.Size())
	for _, validator := range s.ValSet.List() {
		validators = append(validators, validator.Address())
	}
	for i := 0; i < len(validators); i++ {
		for j := i + 1; j < len(validators); j++ {
			if bytes.Compare(validators[i][:], validators[j][:]) > 0 {
				validators[i], validators[j] = validators[j], validators[i]
			}
		}
	}
	return validators
}

type snapshotJSON struct {
	Epoch  uint64                   `json:"epoch"`
	Number uint64                   `json:"number"`
	Hash   common.Hash              `json:"hash"`
	Votes  []*Vote                  `json:"votes"`
	Tally  map[common.Address]Tally `json:"tally"`

	// for validator set
	Validators []common.Address   `json:"validators"`
	Policy     bft.ProposerPolicy `json:"policy"`

	// for dpos
	DposContext *types.DposContext          `json:"dposcontext"`
	StateDB     *state.StateDB              `json:"statedb"`
	//Signers     map[common.Address]struct{} `json:"signers"` // Set of authorized signers at this moment
	//Recents     map[uint64]common.Address   `json:"recents"` // Set of recent signers for spam protections
}

func (s *Snapshot) toJSONStruct() *snapshotJSON {
	return &snapshotJSON{
		Epoch:      s.Epoch,
		Number:     s.Number,
		Hash:       s.Hash,
		Votes:      s.Votes,
		Tally:      s.Tally,
		Validators: s.validators(),
		Policy:     s.ValSet.Policy(),

		DposContext: s.DposContext,
		StateDB:     s.StateDB,
		// Signers:     s.Signers,
		// Recents:     s.Recents,
	}
}

// Unmarshal from a json byte array
func (s *Snapshot) UnmarshalJSON(b []byte) error {
	var j snapshotJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}

	s.Epoch = j.Epoch
	s.Number = j.Number
	s.Hash = j.Hash
	s.Votes = j.Votes
	s.Tally = j.Tally
	s.ValSet = validator.NewSet(j.Validators, j.Policy)

	s.DposContext = j.DposContext
	s.StateDB = j.StateDB
	// s.Signers = j.Signers
	// s.Recents = j.Recents
	return nil
}

// Marshal to a json byte array
func (s *Snapshot) MarshalJSON() ([]byte, error) {
	j := s.toJSONStruct()
	return json.Marshal(j)
}

// countVotes
func (ec *Snapshot) countVotes() (votes map[common.Address]*big.Int, err error) {
	votes = map[common.Address]*big.Int{}
	delegateTrie := ec.DposContext.DelegateTrie()
	candidateTrie := ec.DposContext.CandidateTrie()
	statedb := ec.StateDB

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
		//delegateIterator := trie.NewIterator(delegateTrie.PrefixIterator(candidate))
		delegateIterator := trie.NewIterator(delegateTrie.PrefixIterator(cc.Addr.Bytes()))
		existDelegator := delegateIterator.Next()
		if !existDelegator {
			log.Info("-------!existDelegator", "candidate", candidateAddr)
			votes[candidateAddr] = new(big.Int)
			existCandidate = iterCandidate.Next()
			continue
		}
		for existDelegator {
			log.Info("-------existDelegator", "candidate", candidateAddr)
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
	for k, v := range votes {
		log.Info("votes:", "k", k, "v", v)
	}
	return votes, nil
}

// CommitScores commit candidate score into candidate tree in every epoch elec process
func (ec *Snapshot) CommitScores(scores map[common.Address]*big.Int) error {
	for k, v := range scores {
		cc, err := ec.DposContext.GetCandidateContext(k)
		if err != nil {
			return err
		}
		if bytes.Compare(cc.Addr.Bytes(), k.Bytes()) == 0 {
			cc.Score = v
			err = ec.DposContext.SetCandidateContext(cc)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (ec *Snapshot) kickoutValidator() error {
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
			needKickoutValidators = append(needKickoutValidators, &types.SortableAddress{Address: validator, Weight: big.NewInt(cnt)})
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
func (ec *Snapshot) updateMintCnt(validator common.Address) {
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

func (ec *Snapshot) tryElect(genesis, parent *types.Header) error {

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

	err = ec.CommitScores(votes)
	if err != nil {
		return err
	}

	candidates := types.SortableAddresses{}
	for candidate, cnt := range votes {
		candidates = append(candidates, &types.SortableAddress{Address: candidate, Weight: cnt})
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
