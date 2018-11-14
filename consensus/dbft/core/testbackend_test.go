package core

import (
	"crypto/ecdsa"
	"math/big"
	"time"

	"github.com/linkeye/linkeye/common"
	"github.com/linkeye/linkeye/consensus/dbft"
	"github.com/linkeye/linkeye/consensus/dbft/validator"
	"github.com/linkeye/linkeye/crypto"
	"github.com/linkeye/linkeye/letdb"
	"github.com/linkeye/linkeye/event"
	elog "github.com/linkeye/linkeye/log"
)

var testLogger = elog.New()

type testSystemBackend struct {
	id  uint64
	sys *testSystem

	engine Engine
	peers  bft.ValidatorSet
	events *event.TypeMux

	committedMsgs []testCommittedMsgs
	sentMsgs      [][]byte // store the message when Send is called by core

	address common.Address
	db      letdb.Database
}

type testCommittedMsgs struct {
	commitProposal bft.Proposal
	committedSeals [][]byte
}

// ==============================================
//
// define the functions that needs to be provided for BFT.

func (self *testSystemBackend) Address() common.Address {
	return self.address
}

// Peers returns all connected peers
func (self *testSystemBackend) Validators(proposal bft.Proposal) bft.ValidatorSet {
	return self.peers
}

func (self *testSystemBackend) EventMux() *event.TypeMux {
	return self.events
}

func (self *testSystemBackend) Send(message []byte, target common.Address) error {
	testLogger.Info("enqueuing a message...", "address", self.Address())
	self.sentMsgs = append(self.sentMsgs, message)
	self.sys.queuedMessage <- bft.MessageEvent{
		Payload: message,
	}
	return nil
}

func (self *testSystemBackend) Broadcast(valSet bft.ValidatorSet, message []byte) error {
	testLogger.Info("enqueuing a message...", "address", self.Address())
	self.sentMsgs = append(self.sentMsgs, message)
	self.sys.queuedMessage <- bft.MessageEvent{
		Payload: message,
	}
	return nil
}

func (self *testSystemBackend) Gossip(valSet bft.ValidatorSet, message []byte) error {
	testLogger.Warn("not sign any data")
	return nil
}

func (self *testSystemBackend) Commit(proposal bft.Proposal, seals [][]byte) error {
	testLogger.Info("commit message", "address", self.Address())
	self.committedMsgs = append(self.committedMsgs, testCommittedMsgs{
		commitProposal: proposal,
		committedSeals: seals,
	})

	// fake new head events
	go self.events.Post(bft.FinalCommittedEvent{})
	return nil
}

func (self *testSystemBackend) Verify(proposal bft.Proposal) (time.Duration, error) {
	return 0, nil
}

func (self *testSystemBackend) Sign(data []byte) ([]byte, error) {
	testLogger.Warn("not sign any data")
	return data, nil
}

func (self *testSystemBackend) CheckSignature([]byte, common.Address, []byte) error {
	return nil
}

func (self *testSystemBackend) CheckValidatorSignature(data []byte, sig []byte) (common.Address, error) {
	return common.Address{}, nil
}

func (self *testSystemBackend) Hash(b interface{}) common.Hash {
	return common.StringToHash("Test")
}

func (self *testSystemBackend) NewRequest(request bft.Proposal) {
	go self.events.Post(bft.RequestEvent{
		Proposal: request,
	})
}

func (self *testSystemBackend) HasBadProposal(hash common.Hash) bool {
	return false
}

func (self *testSystemBackend) LastProposal() (bft.Proposal, common.Address) {
	l := len(self.committedMsgs)
	if l > 0 {
		return self.committedMsgs[l-1].commitProposal, common.Address{}
	}
	return makeBlock(0), common.Address{}
}

// Only block height 5 will return true
func (self *testSystemBackend) HasPropsal(hash common.Hash, number *big.Int) bool {
	return number.Cmp(big.NewInt(5)) == 0
}

func (self *testSystemBackend) GetProposer(number uint64) common.Address {
	return common.Address{}
}

func (self *testSystemBackend) ParentValidators(proposal bft.Proposal) bft.ValidatorSet {
	return self.peers
}

// ==============================================
//
// define the struct that need to be provided for integration tests.

type testSystem struct {
	backends []*testSystemBackend

	queuedMessage chan bft.MessageEvent
	quit          chan struct{}
}

func newTestSystem(n uint64) *testSystem {
	testLogger.SetHandler(elog.StdoutHandler)
	return &testSystem{
		backends: make([]*testSystemBackend, n),

		queuedMessage: make(chan bft.MessageEvent),
		quit:          make(chan struct{}),
	}
}

func generateValidators(n int) []common.Address {
	vals := make([]common.Address, 0)
	for i := 0; i < n; i++ {
		privateKey, _ := crypto.GenerateKey()
		vals = append(vals, crypto.PubkeyToAddress(privateKey.PublicKey))
	}
	return vals
}

func newTestValidatorSet(n int) bft.ValidatorSet {
	return validator.NewSet(generateValidators(n), bft.RoundRobin)
}

// FIXME: int64 is needed for N and F
func NewTestSystemWithBackend(n, f uint64) *testSystem {
	testLogger.SetHandler(elog.StdoutHandler)

	addrs := generateValidators(int(n))
	sys := newTestSystem(n)
	config := bft.DefaultConfig

	for i := uint64(0); i < n; i++ {
		vset := validator.NewSet(addrs, bft.RoundRobin)
		backend := sys.NewBackend(i)
		backend.peers = vset
		backend.address = vset.GetByIndex(i).Address()

		core := New(backend, config).(*core)
		core.state = StateAcceptRequest
		core.current = newRoundState(&bft.View{
			Round:    big.NewInt(0),
			Sequence: big.NewInt(1),
		}, vset, common.Hash{}, nil, nil, func(hash common.Hash) bool {
			return false
		})
		core.valSet = vset
		core.logger = testLogger
		core.validateFn = backend.CheckValidatorSignature

		backend.engine = core
	}

	return sys
}

// listen will consume messages from queue and deliver a message to core
func (t *testSystem) listen() {
	for {
		select {
		case <-t.quit:
			return
		case queuedMessage := <-t.queuedMessage:
			testLogger.Info("consuming a queue message...")
			for _, backend := range t.backends {
				go backend.EventMux().Post(queuedMessage)
			}
		}
	}
}

// Run will start system components based on given flag, and returns a closer
// function that caller can control lifecycle
//
// Given a true for core if you want to initialize core engine.
func (t *testSystem) Run(core bool) func() {
	for _, b := range t.backends {
		if core {
			b.engine.Start() // start BFT core
		}
	}

	go t.listen()
	closer := func() { t.stop(core) }
	return closer
}

func (t *testSystem) stop(core bool) {
	close(t.quit)

	for _, b := range t.backends {
		if core {
			b.engine.Stop()
		}
	}
}

func (t *testSystem) NewBackend(id uint64) *testSystemBackend {
	// assume always success
	letDB, _ := letdb.NewMemDatabase()
	backend := &testSystemBackend{
		id:     id,
		sys:    t,
		events: new(event.TypeMux),
		db:     letDB,
	}

	t.backends[id] = backend
	return backend
}

// ==============================================
//
// helper functions.

func getPublicKeyAddress(privateKey *ecdsa.PrivateKey) common.Address {
	return crypto.PubkeyToAddress(privateKey.PublicKey)
}
