package types

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/linkeye/linkeye/common"
	"github.com/linkeye/linkeye/common/hexutil"
	"github.com/linkeye/linkeye/crypto/sha3"
	"github.com/linkeye/linkeye/letdb"
	"github.com/linkeye/linkeye/log"
	"github.com/linkeye/linkeye/rlp"
	"github.com/linkeye/linkeye/trie"
)

type CoinState uint8

const (
	LoginState CoinState = iota
	LogoutState
	WithdrawingState
)

// 候选人抵押结构体，包含候选人地址，币抵押状态（完全抵押，解押中，完全解押）,块号和得票数
type CandidateContext struct {
	Addr        common.Address `json:"addr" gencodec:"required"`
	State       CoinState      `json:"state" gencodec:"required"`
	BlockNumber *big.Int       `json:"blocknumber" gencodec:"required"`
	Score       *big.Int       `json:"score" gencodec:"required"`
}

func (cc *CandidateContext) String() string {
	return fmt.Sprintf("Addr: %v, State: %v, BlockNumber: %v, Score: %v", cc.Addr.String(), cc.State, cc.BlockNumber, cc.Score)
}

// SortCandidateContexts, p[0].score > p[1].score > [p...].score
type SortCandidateContexts []CandidateContext

func (p SortCandidateContexts) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p SortCandidateContexts) Len() int      { return len(p) }
func (p SortCandidateContexts) Less(i, j int) bool {
	if p[i].Score.Cmp(p[j].Score) < 0 {
		return false
	} else if p[i].Score.Cmp(p[j].Score) > 0 {
		return true
	} else {
		return p[i].Addr.String() > p[j].Addr.String()
	}
}

// 投票人结构体，包含投票人地址，候选人地址，投票时块号和投票人当时余额
type VoteContext struct {
	Delegator        common.Address `json:"delegator" gencodec:"required"`
	Candidate        common.Address `json:"candidate" gencodec:"required"`
	BlockNumber      *big.Int       `json:"blocknumber" gencodec:"required"`
	DelegatorBalance *big.Int       `json:"delegatorbalance" gencodec:"required"`
}

// SortVoteContexts, p[0].delegatorbalance > p[1].delegatorbalance > [p...].delegatorbalance
type SortVoteContexts []VoteContext

func (p SortVoteContexts) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p SortVoteContexts) Len() int      { return len(p) }
func (p SortVoteContexts) Less(i, j int) bool {
	if p[i].DelegatorBalance.Cmp(p[j].DelegatorBalance) < 0 {
		return false
	} else if p[i].DelegatorBalance.Cmp(p[j].DelegatorBalance) > 0 {
		return true
	} else {
		return p[i].Delegator.String() > p[j].Delegator.String()
	}
}

type MintCntAddress struct {
	Address common.Address `json:"addr" gencodec:"required"`
	MintCnt int64          `json:"mintcnt" gencodec:"required"`
}

type SortableMintCntAddresses []MintCntAddress

func (p SortableMintCntAddresses) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p SortableMintCntAddresses) Len() int      { return len(p) }
func (p SortableMintCntAddresses) Less(i, j int) bool {
	if p[i].MintCnt < p[j].MintCnt {
		return false
	} else if p[i].MintCnt > p[j].MintCnt {
		return true
	} else {
		return p[i].Address.String() > p[j].Address.String()
	}
}

type SortableAddress struct {
	Address common.Address `json:"addr" gencodec:"required"`
	Weight  *big.Int       `json:"weight" gencodec:"required"`
}
type SortableAddresses []*SortableAddress

func (p SortableAddresses) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p SortableAddresses) Len() int      { return len(p) }
func (p SortableAddresses) Less(i, j int) bool {
	if p[i].Weight.Cmp(p[j].Weight) < 0 {
		return false
	} else if p[i].Weight.Cmp(p[j].Weight) > 0 {
		return true
	} else {
		return p[i].Address.String() < p[j].Address.String()
	}
}

type DposContext struct {
	epochTrie     *trie.Trie
	delegateTrie  *trie.Trie
	voteTrie      *trie.Trie
	candidateTrie *trie.Trie
	mintCntTrie   *trie.Trie

	db *trie.Database
}

var (
	epochPrefix     = []byte("epoch-")
	delegatePrefix  = []byte("delegate-")
	votePrefix      = []byte("vote-")
	candidatePrefix = []byte("candidate-")
	mintCntPrefix   = []byte("mintCnt-")
)

func NewEpochTrie(root common.Hash, db *trie.Database) (*trie.Trie, error) {
	return trie.NewTrieWithPrefix(root, epochPrefix, db)
}

func NewDelegateTrie(root common.Hash, db *trie.Database) (*trie.Trie, error) {
	return trie.NewTrieWithPrefix(root, delegatePrefix, db)
}

func NewVoteTrie(root common.Hash, db *trie.Database) (*trie.Trie, error) {
	return trie.NewTrieWithPrefix(root, votePrefix, db)
}

func NewCandidateTrie(root common.Hash, db *trie.Database) (*trie.Trie, error) {
	return trie.NewTrieWithPrefix(root, candidatePrefix, db)
}

func NewMintCntTrie(root common.Hash, db *trie.Database) (*trie.Trie, error) {
	return trie.NewTrieWithPrefix(root, mintCntPrefix, db)
}

func NewDposContext(db letdb.Database) (*DposContext, error) {

	tdb := trie.NewDatabase(db)
	epochTrie, err := NewEpochTrie(common.Hash{}, tdb)
	if err != nil {
		return nil, err
	}
	delegateTrie, err := NewDelegateTrie(common.Hash{}, tdb)
	if err != nil {
		return nil, err
	}
	voteTrie, err := NewVoteTrie(common.Hash{}, tdb)
	if err != nil {
		return nil, err
	}
	candidateTrie, err := NewCandidateTrie(common.Hash{}, tdb)
	if err != nil {
		return nil, err
	}
	mintCntTrie, err := NewMintCntTrie(common.Hash{}, tdb)
	if err != nil {
		return nil, err
	}
	return &DposContext{
		epochTrie:     epochTrie,
		delegateTrie:  delegateTrie,
		voteTrie:      voteTrie,
		candidateTrie: candidateTrie,
		mintCntTrie:   mintCntTrie,
		db:            tdb,
	}, nil
}

func NewDposContextFromProto(db letdb.Database, ctxProto *DposContextProto) (*DposContext, error) {
	tdb := trie.NewDatabase(db)
	epochTrie, err := NewEpochTrie(ctxProto.EpochHash, tdb)
	if err != nil {
		log.Error("NewEpochTrie:", "error", err)
		return nil, err
	}
	delegateTrie, err := NewDelegateTrie(ctxProto.DelegateHash, tdb)
	if err != nil {
		log.Error("NewDelegateTrie:", "error", err)
		return nil, err
	}
	voteTrie, err := NewVoteTrie(ctxProto.VoteHash, tdb)
	if err != nil {
		log.Error("NewVoteTrie:", "error", err)
		return nil, err
	}
	candidateTrie, err := NewCandidateTrie(ctxProto.CandidateHash, tdb)
	if err != nil {
		log.Error("NewCandidateTrie:", "error", err)
		return nil, err
	}
	mintCntTrie, err := NewMintCntTrie(ctxProto.MintCntHash, tdb)
	if err != nil {
		log.Error("NewMintCntTrie:", "error", err)
		return nil, err
	}
	return &DposContext{
		epochTrie:     epochTrie,
		delegateTrie:  delegateTrie,
		voteTrie:      voteTrie,
		candidateTrie: candidateTrie,
		mintCntTrie:   mintCntTrie,
		db:            tdb,
	}, nil
}

func (d *DposContext) Copy() *DposContext {
	if d.epochTrie == nil || d.delegateTrie == nil || d.voteTrie == nil || d.candidateTrie == nil || d.mintCntTrie == nil || d.db == nil {
		log.Error("exist nil", "d", *d)
		return &DposContext{}
	}
	epochTrie := *d.epochTrie
	delegateTrie := *d.delegateTrie
	voteTrie := *d.voteTrie
	candidateTrie := *d.candidateTrie
	mintCntTrie := *d.mintCntTrie
	db := *d.db
	return &DposContext{
		epochTrie:     &epochTrie,
		delegateTrie:  &delegateTrie,
		voteTrie:      &voteTrie,
		candidateTrie: &candidateTrie,
		mintCntTrie:   &mintCntTrie,
		db: &db,
	}
}

func (d *DposContext) Root() (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, d.epochTrie.Hash())
	rlp.Encode(hw, d.delegateTrie.Hash())
	rlp.Encode(hw, d.candidateTrie.Hash())
	rlp.Encode(hw, d.voteTrie.Hash())
	rlp.Encode(hw, d.mintCntTrie.Hash())
	hw.Sum(h[:0])
	return h
}

func (d *DposContext) Snapshot() *DposContext {
	return d.Copy()
}

func (d *DposContext) RevertToSnapShot(snapshot *DposContext) {
	d.epochTrie = snapshot.epochTrie
	d.delegateTrie = snapshot.delegateTrie
	d.candidateTrie = snapshot.candidateTrie
	d.voteTrie = snapshot.voteTrie
	d.mintCntTrie = snapshot.mintCntTrie
}

func (d *DposContext) FromProto(dcp *DposContextProto) error {
	var err error
	d.epochTrie, err = NewEpochTrie(dcp.EpochHash, d.db)
	if err != nil {
		log.Error("NewEpochTrie:", "error", err)
		return err
	}
	d.delegateTrie, err = NewDelegateTrie(dcp.DelegateHash, d.db)
	if err != nil {
		log.Error("NewDelegateTrie:", "error", err)
		return err
	}
	d.candidateTrie, err = NewCandidateTrie(dcp.CandidateHash, d.db)
	if err != nil {
		log.Error("NewCandidateTrie:", "error", err)
		return err
	}
	d.voteTrie, err = NewVoteTrie(dcp.VoteHash, d.db)
	if err != nil {
		log.Error("NewVoteTrie:", "error", err)
		return err
	}
	d.mintCntTrie, err = NewMintCntTrie(dcp.MintCntHash, d.db)
	if err != nil {
		log.Error("NewMintCntTrie:", "error", err)
		return err
	}
	return nil
}

type DposContextProto struct {
	EpochHash     common.Hash `json:"epochRoot"        gencodec:"required"`
	DelegateHash  common.Hash `json:"delegateRoot"     gencodec:"required"`
	CandidateHash common.Hash `json:"candidateRoot"    gencodec:"required"`
	VoteHash      common.Hash `json:"voteRoot"         gencodec:"required"`
	MintCntHash   common.Hash `json:"mintCntRoot"      gencodec:"required"`
}

func (d *DposContext) ToProto() *DposContextProto {
	return &DposContextProto{
		EpochHash:     d.epochTrie.Hash(),
		DelegateHash:  d.delegateTrie.Hash(),
		CandidateHash: d.candidateTrie.Hash(),
		VoteHash:      d.voteTrie.Hash(),
		MintCntHash:   d.mintCntTrie.Hash(),
	}
}

func (p *DposContextProto) Root() (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, p.EpochHash)
	rlp.Encode(hw, p.DelegateHash)
	rlp.Encode(hw, p.CandidateHash)
	rlp.Encode(hw, p.VoteHash)
	rlp.Encode(hw, p.MintCntHash)
	hw.Sum(h[:0])
	return h
}

func (d *DposContext) KickoutCandidate(candidateAddr common.Address) error {
	candidate := candidateAddr.Bytes()
	err := d.candidateTrie.TryDelete(candidate)
	if err != nil {
		if _, ok := err.(*trie.MissingNodeError); !ok {
			return err
		}
	}
	iter := trie.NewIterator(d.delegateTrie.PrefixIterator(candidate))
	for iter.Next() {
		delegator := iter.Value
		key := append(candidate, delegator...)
		err = d.delegateTrie.TryDelete(key)
		if err != nil {
			if _, ok := err.(*trie.MissingNodeError); !ok {
				return err
			}
		}
		v, err := d.voteTrie.TryGet(delegator)
		if err != nil {
			if _, ok := err.(*trie.MissingNodeError); !ok {
				return err
			}
		}
		if err == nil && bytes.Equal(v, candidate) {
			err = d.voteTrie.TryDelete(delegator)
			if err != nil {
				if _, ok := err.(*trie.MissingNodeError); !ok {
					return err
				}
			}
		}
	}
	return nil
}

func (d *DposContext) BecomeCandidate(candidateAddr common.Address) error {
	candidate := candidateAddr.Bytes()
	return d.candidateTrie.TryUpdate(candidate, candidate)
}

func (d *DposContext) Delegate(delegatorAddr, candidateAddr common.Address, blockNum, delegatorbal *big.Int) error {
	delegator, candidate := delegatorAddr.Bytes(), candidateAddr.Bytes()

	// the candidate must be candidate
	candidateInTrie, err := d.candidateTrie.TryGet(candidate)
	if err != nil {
		return err
	}
	if candidateInTrie == nil {
		return errors.New("invalid candidate to delegate")
	}

	// delete old candidate if exists
	oldCandidateRLP, err := d.voteTrie.TryGet(delegator)
	if err != nil {
		if _, ok := err.(*trie.MissingNodeError); !ok {
			return err
		}
	}
	var vc VoteContext
	if oldCandidateRLP != nil {
		if err := rlp.DecodeBytes(oldCandidateRLP, &vc); err != nil {
			return errors.New("decode " + delegatorAddr.String() + "votecontext error")
		}
		oldCandidate := vc.Candidate.Bytes()
		d.delegateTrie.Delete(append(oldCandidate, delegator...))
	}
	if err = d.delegateTrie.TryUpdate(append(candidate, delegator...), delegator); err != nil {
		return err
	}
	//return d.voteTrie.TryUpdate(delegator, candidate)
	vc.Delegator.SetBytes(delegatorAddr.Bytes())
	vc.Candidate.SetBytes(candidateAddr.Bytes())
	vc.BlockNumber = blockNum
	vc.DelegatorBalance = delegatorbal
	vcRLP, err := rlp.EncodeToBytes(vc)
	if err != nil {
		return errors.New("encode " + delegatorAddr.String() + "to votecontext error")
	}
	return d.voteTrie.TryUpdate(delegator, vcRLP)
}

func (d *DposContext) UnDelegate(delegatorAddr, candidateAddr common.Address) error {
	delegator, candidate := delegatorAddr.Bytes(), candidateAddr.Bytes()

	// the candidate must be candidate
	candidateInTrie, err := d.candidateTrie.TryGet(candidate)
	if err != nil {
		return err
	}
	if candidateInTrie == nil {
		return errors.New("invalid candidate to undelegate")
	}

	oldCandidateRLP, err := d.voteTrie.TryGet(delegator)
	if err != nil {
		return err
	}
	var vc VoteContext
	if oldCandidateRLP != nil {
		if err := rlp.DecodeBytes(oldCandidateRLP, &vc); err != nil {
			return errors.New("decode " + delegatorAddr.String() + "votecontext error")
		}
	}
	if !bytes.Equal(candidate, vc.Candidate.Bytes()) {
		return errors.New("mismatch candidate to undelegate")
	}

	if err = d.delegateTrie.TryDelete(append(candidate, delegator...)); err != nil {
		return err
	}
	return d.voteTrie.TryDelete(delegator)
}

func (d *DposContext) Commit() (*DposContextProto, error) {
	epochRoot, err := d.epochTrie.Commit(nil)
	if err != nil {
		return nil, err
	}
	d.db.Commit(epochRoot, true)
	delegateRoot, err := d.delegateTrie.Commit(nil)
	if err != nil {
		return nil, err
	}
	d.db.Commit(delegateRoot, true)
	voteRoot, err := d.voteTrie.Commit(nil)
	if err != nil {
		return nil, err
	}
	d.db.Commit(voteRoot, true)
	candidateRoot, err := d.candidateTrie.Commit(nil)
	if err != nil {
		return nil, err
	}
	d.db.Commit(candidateRoot, true)
	mintCntRoot, err := d.mintCntTrie.Commit(nil)
	if err != nil {
		return nil, err
	}
	d.db.Commit(mintCntRoot, true)
	return &DposContextProto{
		EpochHash:     epochRoot,
		DelegateHash:  delegateRoot,
		VoteHash:      voteRoot,
		CandidateHash: candidateRoot,
		MintCntHash:   mintCntRoot,
	}, nil
}

func (d *DposContext) CandidateTrie() *trie.Trie          { return d.candidateTrie }
func (d *DposContext) DelegateTrie() *trie.Trie           { return d.delegateTrie }
func (d *DposContext) VoteTrie() *trie.Trie               { return d.voteTrie }
func (d *DposContext) EpochTrie() *trie.Trie              { return d.epochTrie }
func (d *DposContext) MintCntTrie() *trie.Trie            { return d.mintCntTrie }
func (d *DposContext) DB() *trie.Database                 { return d.db }
func (dc *DposContext) SetEpoch(epoch *trie.Trie)         { dc.epochTrie = epoch }
func (dc *DposContext) SetDelegate(delegate *trie.Trie)   { dc.delegateTrie = delegate }
func (dc *DposContext) SetVote(vote *trie.Trie)           { dc.voteTrie = vote }
func (dc *DposContext) SetCandidate(candidate *trie.Trie) { dc.candidateTrie = candidate }
func (dc *DposContext) SetMintCnt(mintCnt *trie.Trie)     { dc.mintCntTrie = mintCnt }

func (dc *DposContext) GetValidators() ([]common.Address, error) {
	var validators []common.Address
	key := []byte("validator")
	validatorsRLP := dc.epochTrie.Get(key)
	if err := rlp.DecodeBytes(validatorsRLP, &validators); err != nil {
		return nil, fmt.Errorf("failed to decode validators: %s", err)
	}
	return validators, nil
}

func (dc *DposContext) SetValidators(validators []common.Address) error {
	key := []byte("validator")
	validatorsRLP, err := rlp.EncodeToBytes(validators)
	if err != nil {
		return fmt.Errorf("failed to encode validators to rlp bytes: %s", err)
	}
	dc.epochTrie.Update(key, validatorsRLP)
	return nil
}

func (dc *DposContext) GetCandidates() ([]CandidateContext, error) {
	candidates := make([]CandidateContext, 0)

	candidateTrie := dc.CandidateTrie()
	iterCandidate := trie.NewIterator(candidateTrie.NodeIterator(nil))
	existCandidate := iterCandidate.Next()
	if !existCandidate {
		return candidates, nil
	}

	for existCandidate {
		var cc CandidateContext
		candidate := iterCandidate.Value
		rlp.DecodeBytes(candidate, &cc)
		candidates = append(candidates, cc)
		existCandidate = iterCandidate.Next()
	}
	return candidates, nil
}
func (dc *DposContext) GetCandidateContext(candidateAddr common.Address) (CandidateContext, error) {
	var cc CandidateContext
	ccRLP := dc.candidateTrie.Get(candidateAddr.Bytes())
	log.Info("GetCandidateContext:", "candidateAddr", candidateAddr.String(), "ccRLP", hexutil.Encode(ccRLP))
	if len(ccRLP) == 0 {
		return cc, nil
	}
	if err := rlp.DecodeBytes(ccRLP, &cc); err != nil {
		log.Error("failed to decode candidate context:", "error", err)
		return cc, fmt.Errorf("failed to decode candidate context: %s", err)
	}
	return cc, nil
}

func (dc *DposContext) SetCandidateContext(cc CandidateContext) error {
	log.Info("SetCandidateContext:", "cc", cc.String())
	ccRLP, err := rlp.EncodeToBytes(cc)
	log.Info("SetCandidateContext:", "key", cc.Addr.String(), "ccRLP", hexutil.Encode(ccRLP))
	if err != nil {
		log.Error("failed to encode candidate contexts to rlp bytes:", "error", err)
		return fmt.Errorf("failed to encode candidate context to rlp bytes: %s", err)
	}
	dc.candidateTrie.Update(cc.Addr.Bytes(), ccRLP)

	return nil
}

func (dc *DposContext) GetSortableAddresses() (SortableAddresses, error) {
	var sa SortableAddresses
	key := []byte("sortvalidator")
	sortedvalidatorsRLP := dc.epochTrie.Get(key)
	if err := rlp.DecodeBytes(sortedvalidatorsRLP, &sa); err != nil {
		return nil, fmt.Errorf("failed to decode sortedvalidators: %s", err)
	}
	return sa, nil
}

func (dc *DposContext) SetSortableAddresses(sa SortableAddresses) error {
	key := []byte("sortvalidator")
	sortedvalidatorsRLP, err := rlp.EncodeToBytes(sa)
	if err != nil {
		return fmt.Errorf("failed to encode sortedvalidators to rlp bytes: %s", err)
	}
	dc.epochTrie.Update(key, sortedvalidatorsRLP)
	return nil
}

func (dc *DposContext) GetMintCnt(validator common.Address) int64 {
	cnt := int64(0)
	cntBytes := dc.mintCntTrie.Get(validator.Bytes())
	if cntBytes != nil {
		cnt = int64(binary.BigEndian.Uint64(cntBytes))
	}
	return cnt
}

func (dc *DposContext) GetMintCnts() ([]MintCntAddress, error) {
	mintCntAddresses := make([]MintCntAddress, 0)

	mintCntTrie := dc.MintCntTrie()
	iterMintCnt := trie.NewIterator(mintCntTrie.NodeIterator(nil))
	existMintCnt := iterMintCnt.Next()
	if !existMintCnt {
		return mintCntAddresses, nil
	}

	for existMintCnt {
		addr := common.Address{}
		addr.SetBytes(iterMintCnt.Key)

		cnt := int64(binary.BigEndian.Uint64(iterMintCnt.Value))

		mintCntAddresses = append(mintCntAddresses, MintCntAddress{addr, cnt})
		existMintCnt = iterMintCnt.Next()
	}
	sort.Sort(SortableMintCntAddresses(mintCntAddresses))
	return mintCntAddresses, nil
}

func (dc *DposContext) GetVote(addr common.Address) (map[string]VoteContext, error) {
	vote := make(map[string]VoteContext)
	candidateRLP := dc.voteTrie.Get(addr.Bytes())
	if candidateRLP != nil {
		var vc VoteContext
		if err := rlp.DecodeBytes(candidateRLP, &vc); err != nil {
			return vote, fmt.Errorf("failed to decode VoteContext: %s", err)
		}
		vote[addr.String()] = vc
	}
	return vote, nil
}

func (dc *DposContext) GetVotes() (map[string]VoteContext, error) {
	vote := make(map[string]VoteContext)

	voteTrie := dc.VoteTrie()
	iterVote := trie.NewIterator(voteTrie.NodeIterator(nil))
	existVote := iterVote.Next()
	if !existVote {
		return vote, nil
	}

	for existVote {
		addrK := common.Address{}
		addrK.SetBytes(iterVote.Key)

		var vc VoteContext
		if err := rlp.DecodeBytes(iterVote.Value, &vc); err != nil {
			return vote, fmt.Errorf("failed to decode VoteContext: %s", err)
		}

		vote[addrK.String()] = vc

		existVote = iterVote.Next()
	}
	return vote, nil
}

// GetCFDs retrieves all candidates from their delegates
func (dc *DposContext) GetCFDs() (map[string]map[string]*big.Int, error) {
	cfd := make(map[string]map[string]*big.Int)
	delegateTrie := dc.DelegateTrie()
	candidateTrie := dc.CandidateTrie()

	iterCandidate := trie.NewIterator(candidateTrie.NodeIterator(nil))
	existCandidate := iterCandidate.Next()
	if !existCandidate {
		log.Error("GetCFDs: no candidates")
		return cfd, errors.New("no candidates")
	}
	for existCandidate {
		var cc CandidateContext
		candidate := iterCandidate.Value
		rlp.DecodeBytes(candidate, &cc)
		candidateAddr := common.BytesToAddress(cc.Addr.Bytes())
		log.Info("loop:", "candidate", candidateAddr)
		//delegateIterator := trie.NewIterator(delegateTrie.PrefixIterator(candidate))
		delegateIterator := trie.NewIterator(delegateTrie.PrefixIterator(cc.Addr.Bytes()))
		existDelegator := delegateIterator.Next()
		if !existDelegator {
			log.Info("!existDelegator:", "candidate", candidateAddr)
			firstMap := cfd[candidateAddr.String()]
			if firstMap == nil {
				firstMap = make(map[string]*big.Int)
				log.Info("firstMap:", "candidate", candidateAddr)
				cfd[candidateAddr.String()] = firstMap
			}
			existCandidate = iterCandidate.Next()
			continue
		}
		for existDelegator {
			firstMap := cfd[candidateAddr.String()]
			if firstMap == nil {
				firstMap = make(map[string]*big.Int)
				log.Info("firstMap In existDelegator:", "candidate", candidateAddr)
				cfd[candidateAddr.String()] = firstMap
			}

			log.Info("existDelegator:", "candidate", candidateAddr)
			delegator := delegateIterator.Value
			delegatorAddr := common.BytesToAddress(delegator)
			log.Info("existDelegator:", "delegate", delegatorAddr)

			score, ok := cfd[candidateAddr.String()][delegatorAddr.String()]
			if !ok {
				score = new(big.Int)
			}

			var vc VoteContext
			candidateRLP := dc.voteTrie.Get(delegator)
			if candidateRLP != nil {
				if err := rlp.DecodeBytes(candidateRLP, &vc); err != nil {
					return cfd, fmt.Errorf("failed to decode VoteContext: %s", err)
				}
			}
			score = vc.DelegatorBalance

			cfd[candidateAddr.String()][delegatorAddr.String()] = score

			existDelegator = delegateIterator.Next()
		}
		existCandidate = iterCandidate.Next()
	}
	return cfd, nil
}
