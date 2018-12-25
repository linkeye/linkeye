package backend

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/linkeye/linkeye/common"
	"github.com/linkeye/linkeye/consensus"
	bft "github.com/linkeye/linkeye/consensus/dbft"
	dbftCore "github.com/linkeye/linkeye/consensus/dbft/core"
	"github.com/linkeye/linkeye/consensus/dbft/validator"
	"github.com/linkeye/linkeye/core"
	"github.com/linkeye/linkeye/core/types"
	"github.com/linkeye/linkeye/crypto"
	"github.com/linkeye/linkeye/event"
	"github.com/linkeye/linkeye/letdb"
	"github.com/linkeye/linkeye/log"
	"github.com/linkeye/linkeye/params"
)

const (
	// fetcherID is the ID indicates the block is from BFT engine
	fetcherID = "dbft"
)

// New creates an Ethereum backend for BFT core engine.
func New(config *bft.Config, privateKey *ecdsa.PrivateKey, db letdb.Database) consensus.DBFT {
	// Allocate the snapshot caches and create the engine
	recents, _ := lru.NewARC(inmemorySnapshots)
	signatures, _ := lru.NewARC(inmemorySignatures) //dpos
	recentMessages, _ := lru.NewARC(inmemoryPeers)
	knownMessages, _ := lru.NewARC(inmemoryMessages)
	backend := &backend{
		config:         config,
		bftEventMux:    new(event.TypeMux),
		privateKey:     privateKey,
		address:        crypto.PubkeyToAddress(privateKey.PublicKey),
		logger:         log.New(),
		db:             db,
		commitCh:       make(chan *types.Block, 1),
		recents:        recents,
		signatures:     signatures,
		candidates:     make(map[common.Address]bool), //alias for dpos proposals
		coreStarted:    false,
		recentMessages: recentMessages,
		knownMessages:  knownMessages,
	}
	backend.logger.Info("New Node:", "privateKeyHex", fmt.Sprintf("%x", privateKey.D), "address", backend.Address())
	backend.core = dbftCore.New(backend, backend.config)
	dposConfig := &params.DPOSConfig {
		Period: config.BlockPeriod,
		Epoch: config.Epoch,
		Validators: config.Validators,
	}
	backend.dpos = NewDPOS(dposConfig, db)

	return backend
}

// ----------------------------------------------------------------------------

type backend struct {
	config       *bft.Config
	bftEventMux  *event.TypeMux
	privateKey   *ecdsa.PrivateKey
	address      common.Address
	core         dbftCore.Engine
	logger       log.Logger
	db           letdb.Database
	chain        consensus.ChainReader
	currentBlock func() *types.Block
	hasBadBlock  func(hash common.Hash) bool

	// the channels for bft engine notifications
	commitCh          chan *types.Block
	proposedBlockHash common.Hash
	sealMu            sync.Mutex
	coreStarted       bool
	coreMu            sync.RWMutex

	// Current list of candidates we are pushing, alias for proposals
	candidates map[common.Address]bool
	// Protects the signer fields
	candidatesLock sync.RWMutex

	recents    *lru.ARCCache // Snapshots for recent block to speed up reorgs
	signatures *lru.ARCCache // Signatures of recent blocks to speed up mining

	// event subscription for ChainHeadEvent event
	broadcaster consensus.Broadcaster

	recentMessages *lru.ARCCache // the cache of peer's messages
	knownMessages  *lru.ARCCache // the cache of self messages

	dpos *DPOS
}

func (sb *backend) DB() letdb.Database { return sb.db }

// Address implements bft.Backend.Address
func (sb *backend) Address() common.Address {
	return sb.address
}

// Validators implements bft.Backend.Validators
func (sb *backend) Validators(proposal bft.Proposal) bft.ValidatorSet {
	return sb.getValidators(proposal.Number().Uint64(), proposal.Hash())
}

// Broadcast implements bft.Backend.Broadcast
func (sb *backend) Broadcast(valSet bft.ValidatorSet, payload []byte) error {
	// send to others
	sb.Gossip(valSet, payload)
	// send to self
	msg := bft.MessageEvent{
		Payload: payload,
	}
	go sb.bftEventMux.Post(msg)
	return nil
}

// Broadcast implements bft.Backend.Gossip
func (sb *backend) Gossip(valSet bft.ValidatorSet, payload []byte) error {
	hash := bft.RLPHash(payload)
	sb.knownMessages.Add(hash, true)

	targets := make(map[common.Address]bool)
	for _, val := range valSet.List() {
		if val.Address() != sb.Address() {
			targets[val.Address()] = true
		}
	}

	if sb.broadcaster != nil && len(targets) > 0 {
		ps := sb.broadcaster.FindPeers(targets)
		for addr, p := range ps {
			ms, ok := sb.recentMessages.Get(addr)
			var m *lru.ARCCache
			if ok {
				m, _ = ms.(*lru.ARCCache)
				if _, k := m.Get(hash); k {
					// This peer had this event, skip it
					continue
				}
			} else {
				m, _ = lru.NewARC(inmemoryMessages)
			}

			m.Add(hash, true)
			sb.recentMessages.Add(addr, m)

			go p.Send(bftMsg, payload)
		}
	}
	return nil
}

// Commit implements bft.Backend.Commit
func (sb *backend) Commit(proposal bft.Proposal, seals [][]byte) error {
	// Check if the proposal is a valid block
	block := &types.Block{}
	block, ok := proposal.(*types.Block)
	if !ok {
		sb.logger.Error("Invalid proposal, %v", proposal)
		log.Error("Commit", "proposal", proposal)
		return errInvalidProposal
	}

	h := block.Header()
	// Append seals into extra-data
	err := writeCommittedSeals(h, seals)
	if err != nil {
		return err
	}
	// update block's header
	block = block.WithSeal(h)

	sb.logger.Info("Committed", "address", sb.Address(), "hash", proposal.Hash(), "number", proposal.Number().Uint64())
	// - if the proposed and committed blocks are the same, send the proposed hash
	//   to commit channel, which is being watched inside the engine.Seal() function.
	// - otherwise, we try to insert the block.
	// -- if success, the ChainHeadEvent event will be broadcasted, try to build
	//    the next block and the previous Seal() will be stopped.
	// -- otherwise, a error will be returned and a round change event will be fired.
	if sb.proposedBlockHash == block.Hash() {
		// feed block hash to Seal() and wait the Seal() result
		sb.commitCh <- block
		return nil
	}

	if sb.broadcaster != nil {
		sb.broadcaster.Enqueue(fetcherID, block)
	}
	return nil
}

// EventMux implements bft.Backend.EventMux
func (sb *backend) EventMux() *event.TypeMux {
	return sb.bftEventMux
}

// Verify implements bft.Backend.Verify
func (sb *backend) Verify(proposal bft.Proposal) (time.Duration, error) {
	// Check if the proposal is a valid block
	block := &types.Block{}
	block, ok := proposal.(*types.Block)
	if !ok {
		sb.logger.Error("Invalid proposal, %v", proposal)
		log.Error("Verify", "proposal", proposal)
		return 0, errInvalidProposal
	}

	// check bad block
	if sb.HasBadProposal(block.Hash()) {
		return 0, core.ErrBlacklistedHash
	}

	// check block body
	txnHash := types.DeriveSha(block.Transactions())
	uncleHash := types.CalcUncleHash(block.Uncles())
	if txnHash != block.Header().TxHash {
		return 0, errMismatchTxhashes
	}
	if uncleHash != nilUncleHash {
		return 0, errInvalidUncleHash
	}

	// verify the header of proposed block
	err := sb.VerifyHeader(sb.chain, block.Header(), false)
	// ignore errEmptyCommittedSeals error because we don't have the committed seals yet
	if err == nil || err == errEmptyCommittedSeals {
		return 0, nil
	} else if err == consensus.ErrFutureBlock {
		return time.Unix(block.Header().Time.Int64(), 0).Sub(now()), consensus.ErrFutureBlock
	}
	return 0, err
}

// Sign implements bft.Backend.Sign
func (sb *backend) Sign(data []byte) ([]byte, error) {
	hashData := crypto.Keccak256([]byte(data))
	sig, err :=  crypto.Sign(hashData, sb.privateKey)
	return sig, err
}

// CheckSignature implements bft.Backend.CheckSignature
func (sb *backend) CheckSignature(data []byte, address common.Address, sig []byte) error {
	signer, err := bft.GetSignatureAddress(data, sig)
	if err != nil {
		log.Error("Failed to get signer address", "err", err)
		return err
	}
	// Compare derived addresses
	if signer != address {
		return errInvalidSignature
	}
	return nil
}

// HasPropsal implements bft.Backend.HashBlock
func (sb *backend) HasPropsal(hash common.Hash, number *big.Int) bool {
	return sb.chain.GetHeader(hash, number.Uint64()) != nil
}

// GetProposer implements bft.Backend.GetProposer
func (sb *backend) GetProposer(number uint64) common.Address {
	if h := sb.chain.GetHeaderByNumber(number); h != nil {
		a, _ := sb.Author(h)
		return a
	}
	return common.Address{}
}

// ParentValidators implements bft.Backend.GetParentValidators
func (sb *backend) ParentValidators(proposal bft.Proposal) bft.ValidatorSet {
	if block, ok := proposal.(*types.Block); ok {
		return sb.getValidators(block.Number().Uint64()-1, block.ParentHash())
	}
	return validator.NewSet(nil, sb.config.ProposerPolicy)
}

func (sb *backend) getValidators(number uint64, hash common.Hash) bft.ValidatorSet {
	snap, err := sb.snapshot(sb.chain, number, hash, nil)
	if err != nil {
		return validator.NewSet(nil, sb.config.ProposerPolicy)
	}
	return snap.ValSet
}

func (sb *backend) LastProposal() (bft.Proposal, common.Address) {
	block := sb.currentBlock()

	var proposer common.Address
	if block.Number().Cmp(common.Big0) > 0 {
		var err error
		proposer, err = sb.Author(block.Header())
		if err != nil {
			log.Error("LastProposal", "proposer", proposer)
			sb.logger.Error("Failed to get block proposer", "err", err)
			return nil, common.Address{}
		}
	}

	// Return header only block here since we don't need block body
	return block, proposer
}

func (sb *backend) HasBadProposal(hash common.Hash) bool {
	if sb.hasBadBlock == nil {
		return false
	}
	return sb.hasBadBlock(hash)
}
