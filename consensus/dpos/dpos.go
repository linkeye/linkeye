// Package dpos implements the Delegated-Proof-of-Stake consensus engine.
package dpos

import (
	"bytes"
	"errors"
	"fmt"
	"hash"
	"math/big"
	"math/rand"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/linkeye/linkeye/accounts"
	"github.com/linkeye/linkeye/common"
	"github.com/linkeye/linkeye/common/hexutil"
	"github.com/linkeye/linkeye/consensus"
	"github.com/linkeye/linkeye/consensus/misc"
	"github.com/linkeye/linkeye/core/state"
	"github.com/linkeye/linkeye/core/types"
	"github.com/linkeye/linkeye/crypto"
	"github.com/linkeye/linkeye/crypto/sha3"
	"github.com/linkeye/linkeye/letdb"
	"github.com/linkeye/linkeye/log"
	"github.com/linkeye/linkeye/params"
	"github.com/linkeye/linkeye/rlp"
	"github.com/linkeye/linkeye/rpc"
)

const (
	inmemoryEpochContexts = 128  // Number of recent vote epochcontexts to keep in memory
	inmemorySignatures    = 4096 // Number of recent block signatures to keep in memory

	wiggleTime = 500 * time.Millisecond // Random delay (per signer) to allow concurrent signers

	maxValidatorSize = 5
	safeSize         = maxValidatorSize*2/3 + 1
	consensusSize    = maxValidatorSize*2/3 + 1
)

// Delegated-Proof-of-Stake protocol constants.
var (
	epochLength = uint64(30000) // Default number of blocks after which to checkpoint and reset the pending votes
	blockPeriod = uint64(15)    // Default minimum difference between two consecutive block's timestamps

	extraVanity = 32 // Fixed number of extra-data prefix bytes reserved for signer vanity
	extraSeal   = 65 // Fixed number of extra-data suffix bytes reserved for signer seal

	nonceAuthVote = hexutil.MustDecode("0xffffffffffffffff") // Magic nonce number to vote on adding a new signer
	nonceDropVote = hexutil.MustDecode("0x0000000000000000") // Magic nonce number to vote on removing a signer.

	uncleHash = types.CalcUncleHash(nil) // Always Keccak256(RLP([])) as uncles are meaningless outside of PoW.

	diffInTurn = big.NewInt(2) // Block difficulty for in-turn signatures
	diffNoTurn = big.NewInt(1) // Block difficulty for out-of-turn signatures
	diffMax    = big.NewInt(5) // Max Block difficulty
)

// Various error messages to mark blocks invalid. These should be private to
// prevent engine specific errors from being referenced in the remainder of the
// codebase, inherently breaking if the engine is swapped out. Please put common
// error types into the consensus package.
var (
	// errUnknownBlock is returned when the list of signers is requested for a block
	// that is not part of the local blockchain.
	errUnknownBlock = errors.New("unknown block")

	// errInvalidCheckpointBeneficiary is returned if a checkpoint/epoch transition
	// block has a beneficiary set to non-zeroes.
	errInvalidCheckpointBeneficiary = errors.New("beneficiary in checkpoint block non-zero")

	// errInvalidVote is returned if a nonce value is something else that the two
	// allowed constants of 0x00..0 or 0xff..f.
	errInvalidVote = errors.New("vote nonce not 0x00..0 or 0xff..f")

	// errInvalidCheckpointVote is returned if a checkpoint/epoch transition block
	// has a vote nonce set to non-zeroes.
	errInvalidCheckpointVote = errors.New("vote nonce in checkpoint block non-zero")

	// errMissingVanity is returned if a block's extra-data section is shorter than
	// 32 bytes, which is required to store the signer vanity.
	errMissingVanity = errors.New("extra-data 32 byte vanity prefix missing")

	// errMissingSignature is returned if a block's extra-data section doesn't seem
	// to contain a 65 byte secp256k1 signature.
	errMissingSignature = errors.New("extra-data 65 byte suffix signature missing")

	// errExtraSigners is returned if non-checkpoint block contain signer data in
	// their extra-data fields.
	errExtraSigners = errors.New("non-checkpoint block contains extra signer list")

	// errInvalidCheckpointSigners is returned if a checkpoint block contains an
	// invalid list of signers (i.e. non divisible by 20 bytes, or not the correct
	// ones).
	errInvalidCheckpointSigners = errors.New("invalid signer list on checkpoint block")

	// errInvalidMixDigest is returned if a block's mix digest is non-zero.
	errInvalidMixDigest = errors.New("non-zero mix digest")

	// errInvalidUncleHash is returned if a block contains an non-empty uncle list.
	errInvalidUncleHash = errors.New("non empty uncle hash")

	// errInvalidDifficulty is returned if the difficulty of a block is not either
	// of 1 or 2, or if the value does not match the turn of the signer.
	errInvalidDifficulty = errors.New("invalid difficulty")

	// ErrInvalidTimestamp is returned if the timestamp of a block is lower than
	// the previous block's timestamp + the minimum block period.
	ErrInvalidTimestamp = errors.New("invalid timestamp")

	// errInvalidVotingChain is returned if an authorization list is attempted to
	// be modified via out-of-range or non-contiguous headers.
	errInvalidVotingChain = errors.New("invalid voting chain")

	// errUnauthorized is returned if a header is signed by a non-authorized entity.
	errUnauthorized = errors.New("unauthorized")

	// errWaitTransactions is returned if an empty block is attempted to be sealed
	// on an instant chain (0 second period). It's important to refuse these as the
	// block reward is zero, so an empty block just bloats the chain... fast.
	errWaitTransactions = errors.New("waiting for transactions")

	ErrWaitForPrevBlock           = errors.New("wait for last block arrived")
	ErrMintFutureBlock            = errors.New("mint the future block")
	ErrMismatchSignerAndValidator = errors.New("mismatch block signer and validator")
	ErrInvalidBlockValidator      = errors.New("invalid block validator")
	ErrInvalidMintBlockTime       = errors.New("invalid time to mint the block")
	ErrNilBlockHeader             = errors.New("nil block header returned")
)

// SignerFn is a signer callback function to request a hash to be signed by a
// backing account.
type SignerFn func(accounts.Account, []byte) ([]byte, error)

// sigHash returns the hash which is used as input for the Delegated-Proof-of-Stake
// signing. It is the hash of the entire header apart from the 65 byte signature
// contained at the end of the extra data.
//
// Note, the method requires the extra data to be at least 65 bytes, otherwise it
// panics. This is done to avoid accidentally using both forms (signature present
// or not), which could be abused to produce different hashes for the same header.
func sigHash(header *types.Header) (hash common.Hash) {
	hasher := sha3.NewKeccak256()

	rlp.Encode(hasher, []interface{}{
		header.ParentHash,
		header.UncleHash,
		header.Validator,
		header.Coinbase,
		header.Root,
		header.TxHash,
		header.ReceiptHash,
		header.Bloom,
		header.Difficulty,
		header.Number,
		header.GasLimit,
		header.GasUsed,
		header.Time,
		header.Extra[:len(header.Extra)-65], // Yes, this will panic if extra is too short
		header.MixDigest,
		header.Nonce,
		header.DposContext.Root(),
	})
	hasher.Sum(hash[:0])
	return hash
}

// ecrecover extracts the Linkeye account address from a signed header.
func ecrecover(header *types.Header, sigcache *lru.ARCCache) (common.Address, error) {
	// If the signature's already cached, return that
	hash := header.Hash()
	if address, known := sigcache.Get(hash); known {
		return address.(common.Address), nil
	}
	// Retrieve the signature from the header extra-data
	if len(header.Extra) < extraSeal {
		return common.Address{}, errMissingSignature
	}
	signature := header.Extra[len(header.Extra)-extraSeal:]

	// Recover the public key and the Linkeye address
	pubkey, err := crypto.Ecrecover(sigHash(header).Bytes(), signature)
	if err != nil {
		return common.Address{}, err
	}
	var signer common.Address
	copy(signer[:], crypto.Keccak256(pubkey[1:])[12:])

	sigcache.Add(hash, signer)
	return signer, nil
}

// dpos is the Delegated-Proof-of-Stake consensus engine
type DPOS struct {
	config *params.DPOSConfig // Consensus engine configuration parameters
	db     letdb.Database     // Database to store and retrieve snapshot checkpoints

	recents    *lru.ARCCache // Epochcontexts for recent block to speed up reorgs
	signatures *lru.ARCCache // Signatures of recent blocks to speed up mining

	proposals map[common.Address]bool // Current list of proposals we are pushing

	signer common.Address // Linkeye address of the signing key
	signFn SignerFn       // Signer function to authorize hashes with
	lock   sync.RWMutex   // Protects the signer fields
}

// New creates a Delegated-Proof-of-Stake consensus engine with the initial
// signers set to the ones provided by the user.
func New(config *params.DPOSConfig, db letdb.Database) *DPOS {
	// Set any missing consensus parameters to their defaults
	conf := *config
	if conf.Epoch == 0 {
		conf.Epoch = epochLength
	}
	// Allocate the snapshot caches and create the engine
	recents, _ := lru.NewARC(inmemoryEpochContexts)
	signatures, _ := lru.NewARC(inmemorySignatures)

	return &DPOS{
		config:     &conf,
		db:         db,
		recents:    recents,
		signatures: signatures,
		proposals:  make(map[common.Address]bool),
	}
}

// Author implements consensus.Engine, returning the Linkeye address recovered
// from the signature in the header's extra-data section.
func (c *DPOS) Author(header *types.Header) (common.Address, error) {
	return ecrecover(header, c.signatures)
}

// VerifyHeader checks whether a header conforms to the consensus rules.
func (c *DPOS) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error {
	return c.verifyHeader(chain, header, nil)
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers. The
// method returns a quit channel to abort the operations and a results channel to
// retrieve the async verifications (the order is that of the input slice).
func (c *DPOS) VerifyHeaders(chain consensus.ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))

	go func() {
		for i, header := range headers {
			err := c.verifyHeader(chain, header, headers[:i])

			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()
	return abort, results
}

// verifyHeader checks whether a header conforms to the consensus rules.The
// caller may optionally pass in a batch of parents (ascending order) to avoid
// looking those up from the database. This is useful for concurrently verifying
// a batch of new headers.
func (c *DPOS) verifyHeader(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {
	if header.Number == nil {
		return errUnknownBlock
	}
	number := header.Number.Uint64()

	// Don't waste time checking blocks from the future
	if header.Time.Cmp(big.NewInt(time.Now().Unix())) > 0 {
		return consensus.ErrFutureBlock
	}
	// Checkpoint blocks need to enforce zero beneficiary
	checkpoint := (number % c.config.Epoch) == 0
	if checkpoint && header.Coinbase != (common.Address{}) {
		return errInvalidCheckpointBeneficiary
	}
	// Nonces must be 0x00..0 or 0xff..f, zeroes enforced on checkpoints
	if !bytes.Equal(header.Nonce[:], nonceAuthVote) && !bytes.Equal(header.Nonce[:], nonceDropVote) {
		return errInvalidVote
	}
	if checkpoint && !bytes.Equal(header.Nonce[:], nonceDropVote) {
		return errInvalidCheckpointVote
	}
	// Check that the extra-data contains both the vanity and signature
	if len(header.Extra) < extraVanity {
		return errMissingVanity
	}
	if len(header.Extra) < extraVanity+extraSeal {
		return errMissingSignature
	}
	// Ensure that the extra-data contains a signer list on checkpoint, but none otherwise
	signersBytes := len(header.Extra) - extraVanity - extraSeal
	if !checkpoint && signersBytes != 0 {
		return errExtraSigners
	}
	if checkpoint && signersBytes%common.AddressLength != 0 {
		return errInvalidCheckpointSigners
	}
	// Ensure that the mix digest is zero as we don't have fork protection currently
	if header.MixDigest != (common.Hash{}) {
		return errInvalidMixDigest
	}
	// Ensure that the block doesn't contain any uncles which are meaningless in DPOS
	if header.UncleHash != uncleHash {
		return errInvalidUncleHash
	}
	// Ensure that the block's difficulty is meaningful (may not be correct at this point)
	/*if number > 0 {
		if header.Difficulty == nil || (header.Difficulty.Cmp(diffInTurn) != 0 && header.Difficulty.Cmp(diffNoTurn) != 0) {
			return errInvalidDifficulty
		}
	}*/

	// If all checks passed, validate any special fields for hard forks
	if err := misc.VerifyForkHashes(chain.Config(), header, false); err != nil {
		return err
	}
	// All basic checks passed, verify cascading fields
	return c.verifyCascadingFields(chain, header, parents)
}

// verifyCascadingFields verifies all the header fields that are not standalone,
// rather depend on a batch of previous headers. The caller may optionally pass
// in a batch of parents (ascending order) to avoid looking those up from the
// database. This is useful for concurrently verifying a batch of new headers.
func (c *DPOS) verifyCascadingFields(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {
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
	if parent.Time.Uint64()+c.config.Period > header.Time.Uint64() {
		return ErrInvalidTimestamp
	}

	// All basic checks passed, verify the seal and return
	return nil
}

// snapshot retrieves the authorization snapshot at a given point in time.
func (c *DPOS) epochContext(chain consensus.ChainReader, number uint64, hash common.Hash, parent *types.Header) (epoch *EpochContext, err error) {
	// Search for a snapshot in memory or on disk for checkpoints

	// If an in-memory snapshot was found, use that
	if s, ok := c.recents.Get(hash); ok {
		epoch = s.(*EpochContext)
		return epoch, nil
	}

	dposContext, err := types.NewDposContextFromProto(c.db, parent.DposContext)
	if err != nil {
		return nil, err
	}

	epoch, err = newEpochContext(number, hash, dposContext)
	if err != nil {
		return nil, err
	}

	// get recent block signers list
	curHeader := chain.GetHeaderByHash(hash)
	if curHeader == nil {
		return nil, ErrNilBlockHeader
	}

	for curHeader.Number.Uint64() > 0 && len(epoch.Recents) <= (len(epoch.Signers)/2+1) {

		epoch.Recents[curHeader.Number.Uint64()] = curHeader.Validator

		curHeader = chain.GetHeaderByHash(curHeader.ParentHash)
		if curHeader == nil {
			return nil, ErrNilBlockHeader
		}
	}

	c.recents.Add(epoch.Hash, epoch)

	return epoch, nil
}

// VerifyUncles implements consensus.Engine, always returning an error for any
// uncles as this consensus mechanism doesn't permit uncles.
func (c *DPOS) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	if len(block.Uncles()) > 0 {
		return errors.New("uncles not allowed")
	}
	return nil
}

// VerifySeal implements consensus.Engine, checking whether the signature contained
// in the header satisfies the consensus protocol requirements.
func (c *DPOS) VerifySeal(chain consensus.ChainReader, header *types.Header) error {
	return c.verifySeal(chain, header, nil)
}

// verifySeal checks whether the signature contained in the header satisfies the
// consensus protocol requirements. The method accepts an optional list of parent
// headers that aren't yet part of the local blockchain to generate the snapshots
// from.
func (c *DPOS) verifySeal(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {

	//log.Info("verifySeal")
	// Verifying the genesis block is not supported
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
	epoch, err := c.epochContext(chain, number-1, header.ParentHash, parent)
	if err != nil {
		return err
	}

	// Resolve the authorization key and check against signers
	signer, err := ecrecover(header, c.signatures)
	if err != nil {
		return err
	}
	if _, ok := epoch.Signers[signer]; !ok {
		return errUnauthorized
	}
	for seen, recent := range epoch.Recents {
		if recent == signer {
			// Signer is among recents, only fail if the current block doesn't shift it out
			if limit := uint64(len(epoch.Signers)/2 + 1); seen > number-limit {
				return errUnauthorized
			}
		}
	}
	// Ensure that the difficulty corresponds to the turn-ness of the signer
	/*inturn := snap.inturn(header.Number.Uint64(), signer)
	if inturn && header.Difficulty.Cmp(diffInTurn) != 0 {
		return errInvalidDifficulty
	}
	if !inturn && header.Difficulty.Cmp(diffNoTurn) != 0 {
		return errInvalidDifficulty
	}*/

	expected := CalcDifficulty(epoch, signer, parent)

	if expected.Cmp(header.Difficulty) != 0 {
		return errInvalidDifficulty
	}

	// Ensure that we have a valid difficulty for the block
	if header.Difficulty.Sign() <= 0 {
		log.Info("header.Difficulty.Sign() <= 0")
		return errInvalidDifficulty
	}

	return nil
}

// Prepare implements consensus.Engine, preparing all the consensus fields of the
// header for running the transactions on top.
func (c *DPOS) Prepare(chain consensus.ChainReader, header *types.Header) error {
	//log.Info("Prepare")
	// If the block isn't a checkpoint, cast a random vote (good enough for now)
	header.Coinbase = common.Address{}
	header.Nonce = types.BlockNonce{}

	number := header.Number.Uint64()

	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}

	// Assemble the voting snapshot to check which votes make sense
	epoch, err := c.epochContext(chain, number-1, header.ParentHash, parent)
	if err != nil {
		return err
	}

	// set Validator
	header.Validator = c.signer

	// Set the correct difficulty
	header.Difficulty = CalcDifficulty(epoch, c.signer, parent)

	// Ensure the extra data has all it's components
	if len(header.Extra) < extraVanity {
		header.Extra = append(header.Extra, bytes.Repeat([]byte{0x00}, extraVanity-len(header.Extra))...)
	}
	header.Extra = header.Extra[:extraVanity]

	header.Extra = append(header.Extra, make([]byte, extraSeal)...)

	// Mix digest is reserved for now, set to empty
	header.MixDigest = common.Hash{}

	header.Time = new(big.Int).Add(parent.Time, new(big.Int).SetUint64(c.config.Period))
	if header.Time.Int64() < time.Now().Unix() {
		header.Time = big.NewInt(time.Now().Unix())
	}
	return nil
}

// Judge the create contract Transaction from a no authorized account for dpos engine.
func (c *DPOS) JudgeTx(chain consensus.ChainReader, header *types.Header, tx *types.Transaction, from common.Address) error {

	// get the number of the new block
	number := header.Number.Uint64()
	//log.Info("PrepareTx number ","number",   number)

	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	// get current block epochcontext
	epoch, err := c.epochContext(chain, number-1, header.ParentHash, parent)
	if err != nil {
		return err
	}
	//log.Info("PrepareTx get snapshot ")
	if tx.To() == nil {
		//log.Info("PrepareTx tx.To() == nil")
		//judge a account is not a authorizer
		if _, authorized := epoch.Signers[from]; !authorized {
			log.Info("PrepareTx create contract no authorized", "from", from)
			return errUnauthorized
		} else {
			log.Info("PrepareTx create contract authorized", "from", from)
		}
	} else {
		//log.Info("PrepareTx tx.To() != nil", "from", from)
	}
	return nil
}

// Finalize implements consensus.Engine, ensuring no uncles are set, nor block
// rewards given, and returns the final block.
func (c *DPOS) Finalize(chain consensus.ChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction, uncles []*types.Header, receipts []*types.Receipt, dposContext *types.DposContext) (*types.Block, error) {
	// No block rewards in DPOS, so the state remains as is and uncles are dropped
	header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	header.UncleHash = types.CalcUncleHash(nil)

	number := header.Number.Uint64()
	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return nil, consensus.ErrUnknownAncestor
	}

	epoch, err := newEpochContext(number-1, header.ParentHash, dposContext)
	if err != nil {
		return nil, err
	}

	// every epoch , reset signer list
	if number%c.config.Epoch == 0 {
		genesis := chain.GetHeaderByNumber(0)
		err = epoch.tryElect(genesis, parent)
		if err != nil {
			return nil, fmt.Errorf("got error when elect next epoch, err: %s", err)
		}
	}

	//update mint count trie
	epoch.updateMintCnt(header.Validator)

	header.DposContext = epoch.DposContext.ToProto()

	// Assemble and return the final block for sealing
	block := types.NewBlock(header, txs, nil, receipts)
	block.DposContext = epoch.DposContext
	return block, nil
}

// Authorize injects a private key into the consensus engine to mint new blocks
// with.
func (c *DPOS) Authorize(signer common.Address, signFn SignerFn) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.signer = signer
	c.signFn = signFn
}

// Seal implements consensus.Engine, attempting to create a sealed block using
// the local signing credentials.
func (c *DPOS) Seal(chain consensus.ChainReader, block *types.Block, stop <-chan struct{}) (*types.Block, error) {
	header := block.Header()

	// Sealing the genesis block is not supported
	number := header.Number.Uint64()
	if number == 0 {
		return nil, errUnknownBlock
	}
	//log.Info("Seal", "number", number)

	// For 0-period chains, refuse to seal empty blocks (no reward but would spin sealing)
	if c.config.Period == 0 && len(block.Transactions()) == 0 {
		return nil, errWaitTransactions
	}
	// Don't hold the signer fields for the entire sealing procedure
	c.lock.RLock()
	signer, signFn := c.signer, c.signFn
	c.lock.RUnlock()

	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return nil, consensus.ErrUnknownAncestor
	}

	// Bail out if we're unauthorized to sign a block
	epoch, err := c.epochContext(chain, number-1, header.ParentHash, parent)
	if err != nil {
		return nil, err
	}
	if _, authorized := epoch.Signers[signer]; !authorized {
		return nil, errUnauthorized
	}
	// If we're amongst the recent signers, wait for the next block
	for seen, recent := range epoch.Recents {
		if recent == signer {
			// Signer is among recents, only wait if the current block doesn't shift it out
			if limit := uint64(len(epoch.Signers)/2 + 1); number < limit || seen > number-limit {
				log.Info("Signed recently,but < signer/2 + 1")
				var period int64
				period = int64(c.config.Period)
				period = period * int64(len(epoch.Signers)/2+1)
				//delay := time.Duration((len(snap.Signers)/2+1) * c.config.Period * time.Second)
				delay := time.Duration(period) * time.Second

				select {
				case <-stop:
					log.Info("Signed recently wait , exit from stop event")
					return nil, nil
				case <-time.After(delay):
					log.Info("Signed recently wait, exit from delay", "delay", delay)
					return nil, consensus.ErrMining
				}

				//return nil, nil
			}
		}
	}
	// Sweet, the protocol permits us to sign the block, wait for our time
	delay := time.Unix(header.Time.Int64(), 0).Sub(time.Now()) // nolint: gosimple
	//if header.Difficulty.Cmp(diffNoTurn) == 0 {
	if !epoch.inturn(number, signer) {
		// It's not our turn explicitly to sign, delay it a bit
		wiggle := time.Duration(len(epoch.Signers)/2+1) * wiggleTime
		delay += time.Duration(rand.Int63n(int64(wiggle)))

		log.Info("Out-of-turn signing requested", "wiggle", common.PrettyDuration(wiggle))
	} else {
		log.Info("In-of-turn signing requested")
	}
	log.Trace("Waiting for slot to sign and propagate", "delay", common.PrettyDuration(delay))

	select {
	case <-stop:
		return nil, nil
	case <-time.After(delay):
	}
	// Sign all the things!
	sighash, err := signFn(accounts.Account{Address: signer}, sigHash(header).Bytes())
	if err != nil {
		return nil, err
	}
	copy(header.Extra[len(header.Extra)-extraSeal:], sighash)

	return block.WithSeal(header), nil
}

// CalcDifficulty is the difficulty adjustment algorithm. It returns the difficulty
// that a new block should have based on the previous blocks in the chain and the
// current signer.
func (c *DPOS) CalcDifficulty(chain consensus.ChainReader, time uint64, parent *types.Header) *big.Int {
	snap, err := c.epochContext(chain, parent.Number.Uint64(), parent.Hash(), nil)
	if err != nil {
		return nil
	}

	return CalcDifficulty(snap, c.signer, parent)
}

// CalcDifficulty is the difficulty adjustment algorithm. It returns the difficulty
// that a new block should have based on the previous blocks in the chain and the
// current signer.
func CalcDifficulty(epoch *EpochContext, signer common.Address, parent *types.Header) *big.Int {
	/*if snap.inturn(snap.Number+1, signer) {
		return new(big.Int).Set(diffInTurn)
	}
	return new(big.Int).Set(diffNoTurn)*/
	if len(epoch.Signers) == 0 {
		return new(big.Int).SetUint64(0)
	}
	soffset := epoch.signerIndex(signer)
	noffset := (epoch.Number + 1) % uint64(len(epoch.Signers))
	var index uint64
	if noffset > soffset {
		index = noffset - soffset
	} else {
		index = uint64(len(epoch.Signers)) - (soffset - noffset)
	}
	//diff = diff * uint64(diffMax) / uint64(len(snap.Signers))
	//diff = diff + parent.Difficulty

	diff := new(big.Int).SetUint64(index)
	signerCount := new(big.Int).SetUint64(uint64(len(epoch.Signers)))
	diff.Mul(diff, diffMax)
	diff.Div(diff, signerCount)
	//diff.Add(diff, parent.Difficulty)
	diff.Add(diff, new(big.Int).SetUint64(1000)) // add a fixed number to avoid wiggle

	//log.Info("CalcDifficulty ","number",   snap.Number+1, "diff", diff)

	return diff
}

// APIs implements consensus.Engine, returning the user facing RPC API to allow
// controlling the signer voting.
func (c *DPOS) APIs(chain consensus.ChainReader) []rpc.API {
	return []rpc.API{{
		Namespace: "dpos",
		Version:   "1.0",
		Service:   &API{chain: chain, dpos: c},
		Public:    false,
	}}
}

// hasher is a repetitive hasher allowing the same hash data structures to be
// reused between hash runs instead of requiring new ones to be created.
type hasher func(dest []byte, data []byte)

// makeHasher creates a repetitive hasher, allowing the same hash data structures
// to be reused between hash runs instead of requiring new ones to be created.
// The returned function is not thread safe!
func makeHasher(h hash.Hash) hasher {
	return func(dest []byte, data []byte) {
		h.Write(data)
		h.Sum(dest[:0])
		h.Reset()
	}
}

// seedHash is the seed to use for generating a verification cache and the mining
// dataset.
func seedHash(block uint64) []byte {
	seed := make([]byte, 32)
	if block < epochLength {
		return seed
	}
	keccak256 := makeHasher(sha3.NewKeccak256())
	for i := 0; i < int(block/epochLength); i++ {
		keccak256(seed, seed)
	}
	return seed
}

// SeedHash is the seed to use for generating a verification cache and the mining
// dataset.
func SeedHash(block uint64) []byte {
	return seedHash(block)
}
