package backend

import (
	"bytes"
	"errors"
	//"fmt"
	"math/big"
	//"math/rand"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/linkeye/linkeye/common"
	"github.com/linkeye/linkeye/common/hexutil"
	"github.com/linkeye/linkeye/consensus"
	bft "github.com/linkeye/linkeye/consensus/dbft"
	dbftCore "github.com/linkeye/linkeye/consensus/dbft/core"
	"github.com/linkeye/linkeye/consensus/dbft/validator"
	"github.com/linkeye/linkeye/core/state"
	"github.com/linkeye/linkeye/core/types"
	"github.com/linkeye/linkeye/crypto/sha3"
	"github.com/linkeye/linkeye/log"
	"github.com/linkeye/linkeye/rlp"
	"github.com/linkeye/linkeye/rpc"
)

const (
	//checkpointInterval = 1024 // Number of blocks after which to save the vote snapshot to the database
	checkpointInterval = 1 // Number of blocks after which to save the vote snapshot to the database
	inmemorySnapshots  = 128  // Number of recent vote snapshots to keep in memory
	inmemorySignatures = 4096 // Number of recent block signatures to keep in memory
	inmemoryPeers      = 40
	inmemoryMessages   = 1024

	maxValidatorSize = 11
	safeSize         = 1
)

var (
	MinDelegateBalance  = big.NewFloat(10 * 1e18)  //10LET
	MinCandidateBalance = big.NewFloat(100 * 1e18) //100LET
	UnlockInterval      = big.NewInt(10)           //10 Block

	//epochLength = uint64(30000) // Default number of blocks after which to checkpoint and reset the pending votes
	epochLength   = uint64(30)
	epochInterval = int64(epochLength) //alias for epochLength for test
)

var (
	// errInvalidProposal is returned when a prposal is malformed.
	errInvalidProposal = errors.New("invalid proposal")
	// errInvalidSignature is returned when given signature is not signed by given
	// address.
	errInvalidSignature = errors.New("invalid signature")
	// errUnknownBlock is returned when the list of validators is requested for a block
	// that is not part of the local blockchain.
	errUnknownBlock = errors.New("unknown block")
	// errUnauthorized is returned if a header is signed by a non authorized entity.
	errUnauthorized = errors.New("unauthorized")
	// errInvalidDifficulty is returned if the difficulty of a block is not 1
	errInvalidDifficulty = errors.New("invalid difficulty")
	// errInvalidExtraDataFormat is returned when the extra data format is incorrect
	errInvalidExtraDataFormat = errors.New("invalid extra data format")
	// errInvalidMixDigest is returned if a block's mix digest is not BFT digest.
	errInvalidMixDigest = errors.New("invalid BFT mix digest")
	// errInvalidNonce is returned if a block's nonce is invalid
	errInvalidNonce = errors.New("invalid nonce")
	// errInvalidUncleHash is returned if a block contains an non-empty uncle list.
	errInvalidUncleHash = errors.New("non empty uncle hash")
	// errInconsistentValidatorSet is returned if the validator set is inconsistent
	errInconsistentValidatorSet = errors.New("non empty uncle hash")
	// errInvalidTimestamp is returned if the timestamp of a block is lower than the previous block's timestamp + the minimum block period.
	errInvalidTimestamp = errors.New("invalid timestamp")
	// errInvalidVotingChain is returned if an authorization list is attempted to
	// be modified via out-of-range or non-contiguous headers.
	errInvalidVotingChain = errors.New("invalid voting chain")
	// errInvalidVote is returned if a nonce value is something else that the two
	// allowed constants of 0x00..0 or 0xff..f.
	errInvalidVote = errors.New("vote nonce not 0x00..0 or 0xff..f")
	// errInvalidCommittedSeals is returned if the committed seal is not signed by any of parent validators.
	errInvalidCommittedSeals = errors.New("invalid committed seals")
	// errEmptyCommittedSeals is returned if the field of committed seals is zero.
	errEmptyCommittedSeals = errors.New("zero committed seals")
	// errMismatchTxhashes is returned if the TxHash in header is mismatch.
	errMismatchTxhashes = errors.New("mismatch transcations hashes")

	ErrMismatchSignerAndValidator = errors.New("mismatch block signer and validator")
	ErrNilBlockHeader             = errors.New("nil block header returned")
)
var (
	defaultDifficulty = big.NewInt(1)
	nilUncleHash      = types.CalcUncleHash(nil) // Always Keccak256(RLP([])) as uncles are meaningless outside of PoW.
	emptyNonce        = types.BlockNonce{}
	now               = time.Now

	nonceAuthVote = hexutil.MustDecode("0xffffffffffffffff") // Magic nonce number to vote on adding a new validator
	nonceDropVote = hexutil.MustDecode("0x0000000000000000") // Magic nonce number to vote on removing a validator.

	inmemoryAddresses  = 20 // Number of recent addresses from ecrecover
	recentAddresses, _ = lru.NewARC(inmemoryAddresses)
)

// Author retrieves the Ethereum address of the account that minted the given
// block, which may be different from the header's coinbase if a consensus
// engine is based on signatures.
func (sb *backend) Author(header *types.Header) (common.Address, error) {
	return ecrecover(header, recentAddresses)
}

// Authorize injects a private key into the consensus engine to mint new blocks
// with.
func (sb *backend) Authorize(signer common.Address, signFn SignerFn) {
	sb.dpos.Authorize(signer, signFn)
}


// VerifyHeader checks whether a header conforms to the consensus rules of a
// given engine. Verifying the seal may be done optionally here, or explicitly
// via the VerifySeal method.
func (sb *backend) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error {
	return sb.verifyHeader(chain, header, nil)
}

// verifyHeader checks whether a header conforms to the consensus rules.The
// caller may optionally pass in a batch of parents (ascending order) to avoid
// looking those up from the database. This is useful for concurrently verifying
// a batch of new headers.
func (sb *backend) verifyHeader(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {
	if header.Number == nil {
		return errUnknownBlock
	}

	// Don't waste time checking blocks from the future
	if header.Time.Cmp(big.NewInt(now().Unix())) > 0 {
		return consensus.ErrFutureBlock
	}

	// Ensure that the extra data format is satisfied
	if _, err := types.ExtractBFTExtra(header); err != nil {
		return errInvalidExtraDataFormat
	}

	/*
	// Ensure that the coinbase is valid
	if header.Nonce != (emptyNonce) && !bytes.Equal(header.Nonce[:], nonceAuthVote) && !bytes.Equal(header.Nonce[:], nonceDropVote) {
		return errInvalidNonce
	}
	*/

	// Ensure that the mix digest is zero as we don't have fork protection currently
	if header.MixDigest != types.BFTDigest {
		return errInvalidMixDigest
	}
	// Ensure that the block doesn't contain any uncles which are meaningless in BFT
	if header.UncleHash != nilUncleHash {
		return errInvalidUncleHash
	}
	// Ensure that the block's difficulty is meaningful (may not be correct at this point)
	if header.Difficulty == nil || header.Difficulty.Cmp(defaultDifficulty) != 0 {
		return errInvalidDifficulty
	}

	// //signer, err := ecrecover(header, c.signatures) //dpos
	// signer, err := ecrecover(header, recentAddresses)
	// if err != nil {
	// 	return err
	// }
	// //check validator
	// if bytes.Compare(signer.Bytes(), header.Coinbase.Bytes()) != 0 {
	// 	return ErrMismatchSignerAndValidator
	// }

	// If all checks passed, validate any special fields for hard forks
//	if err := misc.VerifyForkHashes(chain.Config(), header, false); err != nil {
//		return err
//	}

	return sb.verifyCascadingFields(chain, header, parents)
}

// verifyCascadingFields verifies all the header fields that are not standalone,
// rather depend on a batch of previous headers. The caller may optionally pass
// in a batch of parents (ascending order) to avoid looking those up from the
// database. This is useful for concurrently verifying a batch of new headers.
func (sb *backend) verifyCascadingFields(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {
	// The genesis block is the always valid dead-end
	number := header.Number.Uint64()
	if number == 0 {
		return nil
	}
	// Ensure that the block's timestamp isn't too close to it's parent
	var parent *types.Header
	if len(parents) > 0 {
		parent = parents[len(parents)-1]
	} else {
		parent = chain.GetHeader(header.ParentHash, number-1)
	}
	if parent == nil || parent.Number.Uint64() != number-1 || parent.Hash() != header.ParentHash {
		return consensus.ErrUnknownAncestor
	}
	if parent.Time.Uint64()+sb.config.BlockPeriod > header.Time.Uint64() {
		return errInvalidTimestamp
	}
	// Verify validators in extraData. Validators in snapshot and extraData should be the same.
	snap, err := sb.snapshot(chain, number-1, header.ParentHash, parents)
	if err != nil {
		return err
	}
	validators := make([]byte, len(snap.validators())*common.AddressLength)
	for i, validator := range snap.validators() {
		copy(validators[i*common.AddressLength:], validator[:])
	}
	if err := sb.verifySigner(chain, header, parents); err != nil {
		return err
	}

	return sb.verifyCommittedSeals(chain, header, parents)
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
// concurrently. The method returns a quit channel to abort the operations and
// a results channel to retrieve the async verifications (the order is that of
// the input slice).
func (sb *backend) VerifyHeaders(chain consensus.ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))
	go func() {
		for i, header := range headers {
			err := sb.verifyHeader(chain, header, headers[:i])

			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()
	return abort, results
}

// VerifyUncles verifies that the given block's uncles conform to the consensus
// rules of a given engine.
func (sb *backend) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	if len(block.Uncles()) > 0 {
		return errInvalidUncleHash
	}
	return nil
}

// verifySigner checks whether the signer is in parent's validator set
func (sb *backend) verifySigner(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {
	// Verifying the genesis block is not supported
	number := header.Number.Uint64()
	if number == 0 {
		return errUnknownBlock
	}

	// Retrieve the snapshot needed to verify this header and cache it
	snap, err := sb.snapshot(chain, number-1, header.ParentHash, parents)
	if err != nil {
		return err
	}

	// resolve the authorization key and check against signers
	signer, err := ecrecover(header, recentAddresses)
	if err != nil {
		return err
	}

	// Signer should be in the validator set of previous block's extraData.
	if _, v := snap.ValSet.GetByAddress(signer); v == nil {
		sb.logger.Error("verifySigner", "err", err)
		sb.logger.Info(" ", "signer", signer)
		sb.logger.Info(" ", "valSet", snap.ValSet)
		return errUnauthorized
	}
	return nil
}

// verifyCommittedSeals checks whether every committed seal is signed by one of the parent's validators
func (sb *backend) verifyCommittedSeals(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {
	number := header.Number.Uint64()
	// We don't need to verify committed seals in the genesis block
	if number == 0 {
		return nil
	}

	// Retrieve the snapshot needed to verify this header and cache it
	snap, err := sb.snapshot(chain, number-1, header.ParentHash, parents)
	if err != nil {
		return err
	}

	extra, err := types.ExtractBFTExtra(header)
	if err != nil {
		return err
	}
	// The length of Committed seals should be larger than 0
	if len(extra.CommittedSeal) == 0 {
		return errEmptyCommittedSeals
	}

	validators := snap.ValSet.Copy()
	// Check whether the committed seals are generated by parent's validators
	validSeal := 0
	proposalSeal := dbftCore.PrepareCommittedSeal(header.Hash())
	// 1. Get committed seals from current header
	for _, seal := range extra.CommittedSeal {
		// 2. Get the original address by seal and parent block hash
		addr, err := bft.GetSignatureAddress(proposalSeal, seal)
		if err != nil {
			sb.logger.Error("not a valid address", "err", err)
			return errInvalidSignature
		}
		// Every validator can have only one seal. If more than one seals are signed by a
		// validator, the validator cannot be found and errInvalidCommittedSeals is returned.
		if validators.RemoveValidator(addr) {
			validSeal += 1
		} else {
			return errInvalidCommittedSeals
		}
	}

	// The length of validSeal should be larger than number of faulty node + 1
	if validSeal <= 2*snap.ValSet.F() {
		return errInvalidCommittedSeals
	}

	return nil
}

// VerifySeal checks whether the crypto seal on a header is valid according to
// the consensus rules of the given engine.
func (sb *backend) VerifySeal(chain consensus.ChainReader, header *types.Header) error {
	return sb.verifySeal(chain, header, nil)
}

func (sb *backend) verifySeal(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {
	// get parent header and ensure the signer is in parent's validator set
	number := header.Number.Uint64()
	if number == 0 {
		return errUnknownBlock
	}

	var parent *types.Header
	if len(parents) > 0 {
		parent = parents[len(parents)-1]
	} else {
		parent = chain.GetHeader(header.ParentHash, number-1)
	}
	if parent == nil || parent.Number.Uint64() != number-1 || parent.Hash() != header.ParentHash {
		return consensus.ErrUnknownAncestor
	}
	// Retrieve the epochcontext needed to verify this header and cache it
	// epoch, err := sb.snapshot(chain, number-1, header.ParentHash, parents)
	_, err := sb.snapshot(chain, number-1, header.ParentHash, parents)
	if err != nil {
		return err
	}

	// // Resolve the authorization key and check against signers
	// //signer, err := ecrecover(header, c.signatures)
	// signer, err := ecrecover(header, recentAddresses)
	// if err != nil {
	// 	return err
	// }
	// if _, ok := epoch.Signers[signer]; !ok {
	// 	return errUnauthorized
	// }
	// for seen, recent := range epoch.Recents {
	// 	if recent == signer {
	// 		// Signer is among recents, only fail if the current block doesn't shift it out
	// 		if limit := uint64(len(epoch.Signers)/2 + 1); seen > number-limit {
	// 			return errUnauthorized
	// 		}
	// 	}
	// }

	// ensure that the difficulty equals to defaultDifficulty
	if header.Difficulty.Cmp(defaultDifficulty) != 0 {
		return errInvalidDifficulty
	}
	return sb.verifySigner(chain, header, nil)
}

// Prepare initializes the consensus fields of a block header according to the
// rules of a particular engine. The changes are executed inline.
func (sb *backend) Prepare(chain consensus.ChainReader, header *types.Header) error {
	// unused fields, force to set to empty
	header.Coinbase = common.Address{}
	header.Nonce = emptyNonce
	header.MixDigest = types.BFTDigest

	// copy the parent extra data as the header extra data
	number := header.Number.Uint64()
	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	// use the same difficulty for all blocks
	header.Difficulty = defaultDifficulty

	// Assemble the voting snapshot
	snap, err := sb.snapshot(chain, number-1, header.ParentHash, nil)
	if err != nil {
		return err
	}

	/*
	// get valid candidate list
	sb.candidatesLock.RLock()
	var addresses []common.Address
	var authorizes []bool
	for address, authorize := range sb.candidates {
		if snap.checkVote(address, authorize) {
			addresses = append(addresses, address)
			authorizes = append(authorizes, authorize)
		}
	}
	sb.candidatesLock.RUnlock()

	// pick one of the candidates randomly
	if len(addresses) > 0 {
		index := rand.Intn(len(addresses))
		// add validator voting in coinbase
		header.Coinbase = addresses[index]
		if authorizes[index] {
			copy(header.Nonce[:], nonceAuthVote)
		} else {
			copy(header.Nonce[:], nonceDropVote)
		}
	}
	*/

	// add validators in snapshot to extraData's validators section
	extra, err := prepareExtra(header, snap.validators())
	if err != nil {
		return err
	}
	header.Extra = extra

	// set header's timestamp
	header.Time = new(big.Int).Add(parent.Time, new(big.Int).SetUint64(sb.config.BlockPeriod))
	if header.Time.Int64() < time.Now().Unix() {
		header.Time = big.NewInt(time.Now().Unix())
	}
	return nil
}

// Judge the create contract Transaction from a no authorized account for clique engine.
func (sb *backend) JudgeTx(chain consensus.ChainReader, header *types.Header, tx *types.Transaction, from common.Address) error {
	/*
		// get the number of the new block
		number := header.Number.Uint64()
		//log.Info("PrepareTx number ","number",   number)
		// get current block sanpshot
		snap, err := sb.snapshot(chain, number-1, header.ParentHash, nil)
		if err != nil {
			return err
		}
		//log.Info("PrepareTx get snapshot ")
		if tx.To() == nil {
			//log.Info("PrepareTx tx.To() == nil")
			//judge a account is not a authorizer
			if _, authorized := snap.Signers[from]; !authorized {
				log.Info("PrepareTx create contract no authorized", "from", from)
				return errUnauthorized
			} else {
				log.Info("PrepareTx create contract authorized", "from", from)
			}
		} else {
			//log.Info("PrepareTx tx.To() != nil", "from", from)
		}*/
	return nil
}

// Finalize runs any post-transaction state modifications (e.g. block rewards)
// and assembles the final block.
//
// Note, the block header and state database might be updated to reflect any
// consensus rules that happen at finalization (e.g. block rewards).
func (sb *backend) Finalize(chain consensus.ChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction,
	uncles []*types.Header, receipts []*types.Receipt, dposContext *types.DposContext) (*types.Block, error) {
	// No block rewards in BFT, so the state remains as is and uncles are dropped
	/*
	header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	header.UncleHash = nilUncleHash

	number := header.Number.Uint64()
	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return nil, consensus.ErrUnknownAncestor
	}

	header.DposContext = dposContext.ToProto()

	// Assemble and return the final block for sealing
	block := types.NewBlock(header, txs, nil, receipts)
	block.DposContext = dposContext
	return block, nil
	*/
	return sb.dpos.Finalize(chain, header, state, txs, uncles, receipts, dposContext)
}

// Seal generates a new block for the given input block with the local miner's
// seal place on top.
func (sb *backend) Seal(chain consensus.ChainReader, block *types.Block, stop <-chan struct{}) (*types.Block, error) {
	// update the block header timestamp and signature and propose the block to core engine
	header := block.Header()
	number := header.Number.Uint64()

	// Bail out if we're unauthorized to sign a block
	snap, err := sb.snapshot(chain, number-1, header.ParentHash, nil)
	if err != nil {
		return nil, err
	}
	if _, v := snap.ValSet.GetByAddress(sb.address); v == nil {
		return nil, errUnauthorized
	}

	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return nil, consensus.ErrUnknownAncestor
	}
	block, err = sb.updateBlock(parent, block)
	if err != nil {
		return nil, err
	}

	// wait for the timestamp of header, use this to adjust the block period
	delay := time.Unix(block.Header().Time.Int64(), 0).Sub(now())
	select {
	case <-time.After(delay):
	case <-stop:
		return nil, nil
	}

	// get the proposed block hash and clear it if the seal() is completed.
	sb.sealMu.Lock()
	sb.proposedBlockHash = block.Hash()
	clear := func() {
		sb.proposedBlockHash = common.Hash{}
		sb.sealMu.Unlock()
	}
	defer clear()

	// post block into BFT engine
	go sb.EventMux().Post(bft.RequestEvent{
		Proposal: block,
	})

	for {
		select {
		case result := <-sb.commitCh:
			// if the block hash and the hash from channel are the same,
			// return the result. Otherwise, keep waiting the next hash.
			if block.Hash() == result.Hash() {
				return result, nil
			}
		case <-stop:
			return nil, nil
		}
	}
}

// update timestamp and signature of the block based on its number of transactions
func (sb *backend) updateBlock(parent *types.Header, block *types.Block) (*types.Block, error) {
	header := block.Header()
	// sign the hash
	seal, err := sb.Sign(sigHash(header).Bytes())
	if err != nil {
		return nil, err
	}

	err = writeSeal(header, seal)
	if err != nil {
		return nil, err
	}

	return block.WithSeal(header), nil
}

// CalcDifficulty is the difficulty adjustment algorithm. It returns the difficulty
// that a new block should have based on the previous blocks in the chain and the
// current signer.
func (sb *backend) CalcDifficulty(chain consensus.ChainReader, time uint64, parent *types.Header) *big.Int {

	return defaultDifficulty
}

// APIs returns the RPC APIs this consensus engine provides.
func (sb *backend) APIs(chain consensus.ChainReader) []rpc.API {
	return []rpc.API{{
		Namespace: "dbft",
		Version:   "1.0",
		Service:   &API{chain: chain, bft: sb},
		Public:    true,
	}}
}

// Start implements consensus.BFT.Start
func (sb *backend) Start(chain consensus.ChainReader, currentBlock func() *types.Block, hasBadBlock func(hash common.Hash) bool) error {
	sb.coreMu.Lock()
	defer sb.coreMu.Unlock()
	if sb.coreStarted {
		return bft.ErrStartedEngine
	}

	// clear previous data
	sb.proposedBlockHash = common.Hash{}
	if sb.commitCh != nil {
		close(sb.commitCh)
	}
	sb.commitCh = make(chan *types.Block, 1)

	sb.chain = chain
	sb.currentBlock = currentBlock
	sb.hasBadBlock = hasBadBlock

	if err := sb.core.Start(); err != nil {
		return err
	}

	sb.coreStarted = true
	return nil
}

// Stop implements consensus.BFT.Stop
func (sb *backend) Stop() error {
	sb.coreMu.Lock()
	defer sb.coreMu.Unlock()
	if !sb.coreStarted {
		return bft.ErrStoppedEngine
	}
	if err := sb.core.Stop(); err != nil {
		return err
	}
	sb.coreStarted = false
	return nil
}

// snapshot retrieves the authorization snapshot at a given point in time.
func (sb *backend) snapshot(chain consensus.ChainReader, number uint64, hash common.Hash, parents []*types.Header) (*Snapshot, error) {
	// Search for a snapshot in memory or on disk for checkpoints
	var (
		headers []*types.Header
		snap    *Snapshot
	)
	for snap == nil {
		// If an in-memory snapshot was found, use that
		if s, ok := sb.recents.Get(hash); ok {
			snap = s.(*Snapshot)
			break
		}
		// If an on-disk checkpoint snapshot can be found, use that
		if number%checkpointInterval == 0 {
			if s, err := loadSnapshot(sb.config.Epoch, sb.db, hash); err == nil {
				log.Trace("Loaded voting snapshot form disk", "number", number, "hash", hash)
				snap = s
				break
			}
		}
		// If we're at block zero, make a snapshot
		if number == 0 {
			genesis := chain.GetHeaderByNumber(0)
			if err := sb.VerifyHeader(chain, genesis, false); err != nil {
				return nil, err
			}
			bftExtra, err := types.ExtractBFTExtra(genesis)
			if err != nil {
				return nil, err
			}

			dposContext, err := types.NewDposContextFromProto(sb.db, genesis.DposContext)
			if err != nil {
				return nil, err
			}

			snap = newSnapshot(sb.config.Epoch, 0, genesis.Hash(), validator.NewSet(bftExtra.Validators, sb.config.ProposerPolicy), dposContext, nil)
			if err := snap.store(sb.db); err != nil {
				return nil, err
			}
			log.Trace("Stored genesis voting snapshot to disk")
			break
		}
		// No snapshot for this header, gather the header and move backward
		var header *types.Header
		if len(parents) > 0 {
			// If we have explicit parents, pick from there (enforced)
			header = parents[len(parents)-1]
			if header.Hash() != hash || header.Number.Uint64() != number {
				return nil, consensus.ErrUnknownAncestor
			}
			parents = parents[:len(parents)-1]
		} else {
			// No explicit parents (or no more left), reach out to the database
			header = chain.GetHeader(hash, number)
			if header == nil {
				return nil, consensus.ErrUnknownAncestor
			}
		}

		//FIXME:
		if number > 0 {
			dposContext, err := types.NewDposContextFromProto(sb.db, header.DposContext)
			if err != nil {
				return nil, err
			}

			bftExtra, err := types.ExtractBFTExtra(header)
			if err != nil {
				return nil, err
			}
			snap = newSnapshot(sb.config.Epoch, number-1, header.Hash(), validator.NewSet(bftExtra.Validators, sb.config.ProposerPolicy), dposContext, nil)
		}

		headers = append(headers, header)
		number, hash = number-1, header.ParentHash
	}
	// Previous snapshot found, apply any pending headers on top of it
	for i := 0; i < len(headers)/2; i++ {
		headers[i], headers[len(headers)-1-i] = headers[len(headers)-1-i], headers[i]
	}



	snap, err := snap.apply(headers)
	if err != nil {
		return nil, err
	}
	sb.recents.Add(snap.Hash, snap)

	// // get recent block signers list
	// curHeader := chain.GetHeaderByHash(hash)
	// if curHeader == nil {
	// 	return nil, ErrNilBlockHeader
	// }

	// for curHeader.Number.Uint64() > 0 && len(snap.Recents) < (len(snap.Signers)/2+1) && (curHeader.Number.Uint64()%sb.config.Epoch) != 0 {

	// 	snap.Recents[curHeader.Number.Uint64()] = curHeader.Coinbase

	// 	curHeader = chain.GetHeaderByHash(curHeader.ParentHash)
	// 	if curHeader == nil {
	// 		return nil, ErrNilBlockHeader
	// 	}
	// }

	// If we've generated a new checkpoint snapshot, save to disk
	if snap.Number%checkpointInterval == 0 && len(headers) > 0 {
		if err = snap.store(sb.db); err != nil {
			return nil, err
		}
		log.Trace("Stored voting snapshot to disk", "number", snap.Number, "hash", snap.Hash)
	}
	return snap, err
}

// FIXME: Need to update this for BFT
// sigHash returns the hash which is used as input for the BFT
// signing. It is the hash of the entire header apart from the 65 byte signature
// contained at the end of the extra data.
//
// Note, the method requires the extra data to be at least 65 bytes, otherwise it
// panics. This is done to avoid accidentally using both forms (signature present
// or not), which could be abused to produce different hashes for the same header.
func sigHash(header *types.Header) (hash common.Hash) {
	hasher := sha3.NewKeccak256()

	// Clean seal is required for calculating proposer seal.
	rlp.Encode(hasher, types.BFTFilteredHeader(header, false))
	hasher.Sum(hash[:0])
	return hash
}

// ecrecover extracts the Ethereum account address from a signed header.
func ecrecover(header *types.Header, sigcache *lru.ARCCache) (common.Address, error) {
	hash := header.Hash()
	if addr, ok := sigcache.Get(hash); ok {
		return addr.(common.Address), nil
	}

	// Retrieve the signature from the header extra-data
	bftExtra, err := types.ExtractBFTExtra(header)
	if err != nil {
		return common.Address{}, err
	}

	addr, err := bft.GetSignatureAddress(sigHash(header).Bytes(), bftExtra.Seal)
	if err != nil {
		return addr, err
	}
	sigcache.Add(hash, addr)
	return addr, nil
}

// prepareExtra returns a extra-data of the given header and validators
func prepareExtra(header *types.Header, vals []common.Address) ([]byte, error) {
	var buf bytes.Buffer

	// compensate the lack bytes if header.Extra is not enough BFTExtraVanity bytes.
	if len(header.Extra) < types.BFTExtraVanity {
		header.Extra = append(header.Extra, bytes.Repeat([]byte{0x00}, types.BFTExtraVanity-len(header.Extra))...)
	}
	buf.Write(header.Extra[:types.BFTExtraVanity])

	ist := &types.BFTExtra{
		Validators:    vals,
		Seal:          []byte{},
		CommittedSeal: [][]byte{},
	}

	payload, err := rlp.EncodeToBytes(&ist)
	if err != nil {
		return nil, err
	}

	return append(buf.Bytes(), payload...), nil
}

// writeSeal writes the extra-data field of the given header with the given seals.
// suggest to rename to writeSeal.
func writeSeal(h *types.Header, seal []byte) error {
	if len(seal)%types.BFTExtraSeal != 0 {
		return errInvalidSignature
	}

	bftExtra, err := types.ExtractBFTExtra(h)
	if err != nil {
		return err
	}

	bftExtra.Seal = seal
	payload, err := rlp.EncodeToBytes(&bftExtra)
	if err != nil {
		return err
	}

	h.Extra = append(h.Extra[:types.BFTExtraVanity], payload...)
	return nil
}

// writeCommittedSeals writes the extra-data field of a block header with given committed seals.
func writeCommittedSeals(h *types.Header, committedSeals [][]byte) error {
	if len(committedSeals) == 0 {
		return errInvalidCommittedSeals
	}

	for _, seal := range committedSeals {
		if len(seal) != types.BFTExtraSeal {
			return errInvalidCommittedSeals
		}
	}

	bftExtra, err := types.ExtractBFTExtra(h)
	if err != nil {
		return err
	}

	bftExtra.CommittedSeal = make([][]byte, len(committedSeals))
	copy(bftExtra.CommittedSeal, committedSeals)

	payload, err := rlp.EncodeToBytes(&bftExtra)
	if err != nil {
		return err
	}

	h.Extra = append(h.Extra[:types.BFTExtraVanity], payload...)
	return nil
}

