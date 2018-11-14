package core

import (
	"math/big"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/linkeye/linkeye/common"
	"github.com/linkeye/linkeye/consensus/dbft"
	"github.com/linkeye/linkeye/consensus/dbft/validator"
	"github.com/linkeye/linkeye/event"
	"github.com/linkeye/linkeye/log"
	"gopkg.in/karalabe/cookiejar.v2/collections/prque"
)

func TestCheckMessage(t *testing.T) {
	c := &core{
		state: StateAcceptRequest,
		current: newRoundState(&bft.View{
			Sequence: big.NewInt(1),
			Round:    big.NewInt(0),
		}, newTestValidatorSet(4), common.Hash{}, nil, nil, nil),
	}

	// invalid view format
	err := c.checkMessage(msgPreprepare, nil)
	if err != errInvalidMessage {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidMessage)
	}

	testStates := []State{StateAcceptRequest, StatePreprepared, StatePrepared, StateCommitted}
	testCode := []uint64{msgPreprepare, msgPrepare, msgCommit, msgRoundChange}

	// future sequence
	v := &bft.View{
		Sequence: big.NewInt(2),
		Round:    big.NewInt(0),
	}
	for i := 0; i < len(testStates); i++ {
		c.state = testStates[i]
		for j := 0; j < len(testCode); j++ {
			err := c.checkMessage(testCode[j], v)
			if err != errFutureMessage {
				t.Errorf("error mismatch: have %v, want %v", err, errFutureMessage)
			}
		}
	}

	// future round
	v = &bft.View{
		Sequence: big.NewInt(1),
		Round:    big.NewInt(1),
	}
	for i := 0; i < len(testStates); i++ {
		c.state = testStates[i]
		for j := 0; j < len(testCode); j++ {
			err := c.checkMessage(testCode[j], v)
			if testCode[j] == msgRoundChange {
				if err != nil {
					t.Errorf("error mismatch: have %v, want nil", err)
				}
			} else if err != errFutureMessage {
				t.Errorf("error mismatch: have %v, want %v", err, errFutureMessage)
			}
		}
	}

	// current view but waiting for round change
	v = &bft.View{
		Sequence: big.NewInt(1),
		Round:    big.NewInt(0),
	}
	c.waitingForRoundChange = true
	for i := 0; i < len(testStates); i++ {
		c.state = testStates[i]
		for j := 0; j < len(testCode); j++ {
			err := c.checkMessage(testCode[j], v)
			if testCode[j] == msgRoundChange {
				if err != nil {
					t.Errorf("error mismatch: have %v, want nil", err)
				}
			} else if err != errFutureMessage {
				t.Errorf("error mismatch: have %v, want %v", err, errFutureMessage)
			}
		}
	}
	c.waitingForRoundChange = false

	v = c.currentView()
	// current view, state = StateAcceptRequest
	c.state = StateAcceptRequest
	for i := 0; i < len(testCode); i++ {
		err = c.checkMessage(testCode[i], v)
		if testCode[i] == msgRoundChange {
			if err != nil {
				t.Errorf("error mismatch: have %v, want nil", err)
			}
		} else if testCode[i] == msgPreprepare {
			if err != nil {
				t.Errorf("error mismatch: have %v, want nil", err)
			}
		} else {
			if err != errFutureMessage {
				t.Errorf("error mismatch: have %v, want %v", err, errFutureMessage)
			}
		}
	}

	// current view, state = StatePreprepared
	c.state = StatePreprepared
	for i := 0; i < len(testCode); i++ {
		err = c.checkMessage(testCode[i], v)
		if testCode[i] == msgRoundChange {
			if err != nil {
				t.Errorf("error mismatch: have %v, want nil", err)
			}
		} else if err != nil {
			t.Errorf("error mismatch: have %v, want nil", err)
		}
	}

	// current view, state = StatePrepared
	c.state = StatePrepared
	for i := 0; i < len(testCode); i++ {
		err = c.checkMessage(testCode[i], v)
		if testCode[i] == msgRoundChange {
			if err != nil {
				t.Errorf("error mismatch: have %v, want nil", err)
			}
		} else if err != nil {
			t.Errorf("error mismatch: have %v, want nil", err)
		}
	}

	// current view, state = StateCommitted
	c.state = StateCommitted
	for i := 0; i < len(testCode); i++ {
		err = c.checkMessage(testCode[i], v)
		if testCode[i] == msgRoundChange {
			if err != nil {
				t.Errorf("error mismatch: have %v, want nil", err)
			}
		} else if err != nil {
			t.Errorf("error mismatch: have %v, want nil", err)
		}
	}

}

func TestStoreBacklog(t *testing.T) {
	c := &core{
		logger:     log.New("backend", "test", "id", 0),
		backlogs:   make(map[bft.Validator]*prque.Prque),
		backlogsMu: new(sync.Mutex),
	}
	v := &bft.View{
		Round:    big.NewInt(10),
		Sequence: big.NewInt(10),
	}
	p := validator.New(common.StringToAddress("12345667890"))
	// push preprepare msg
	preprepare := &bft.Preprepare{
		View:     v,
		Proposal: makeBlock(1),
	}
	prepreparePayload, _ := Encode(preprepare)
	m := &message{
		Code: msgPreprepare,
		Msg:  prepreparePayload,
	}
	c.storeBacklog(m, p)
	msg := c.backlogs[p].PopItem()
	if !reflect.DeepEqual(msg, m) {
		t.Errorf("message mismatch: have %v, want %v", msg, m)
	}

	// push prepare msg
	subject := &bft.Subject{
		View:   v,
		Digest: common.StringToHash("1234567890"),
	}
	subjectPayload, _ := Encode(subject)

	m = &message{
		Code: msgPrepare,
		Msg:  subjectPayload,
	}
	c.storeBacklog(m, p)
	msg = c.backlogs[p].PopItem()
	if !reflect.DeepEqual(msg, m) {
		t.Errorf("message mismatch: have %v, want %v", msg, m)
	}

	// push commit msg
	m = &message{
		Code: msgCommit,
		Msg:  subjectPayload,
	}
	c.storeBacklog(m, p)
	msg = c.backlogs[p].PopItem()
	if !reflect.DeepEqual(msg, m) {
		t.Errorf("message mismatch: have %v, want %v", msg, m)
	}

	// push roundChange msg
	m = &message{
		Code: msgRoundChange,
		Msg:  subjectPayload,
	}
	c.storeBacklog(m, p)
	msg = c.backlogs[p].PopItem()
	if !reflect.DeepEqual(msg, m) {
		t.Errorf("message mismatch: have %v, want %v", msg, m)
	}
}

func TestProcessFutureBacklog(t *testing.T) {
	backend := &testSystemBackend{
		events: new(event.TypeMux),
	}
	c := &core{
		logger:     log.New("backend", "test", "id", 0),
		backlogs:   make(map[bft.Validator]*prque.Prque),
		backlogsMu: new(sync.Mutex),
		backend:    backend,
		current: newRoundState(&bft.View{
			Sequence: big.NewInt(1),
			Round:    big.NewInt(0),
		}, newTestValidatorSet(4), common.Hash{}, nil, nil, nil),
		state: StateAcceptRequest,
	}
	c.subscribeEvents()
	defer c.unsubscribeEvents()

	v := &bft.View{
		Round:    big.NewInt(10),
		Sequence: big.NewInt(10),
	}
	p := validator.New(common.StringToAddress("12345667890"))
	// push a future msg
	subject := &bft.Subject{
		View:   v,
		Digest: common.StringToHash("1234567890"),
	}
	subjectPayload, _ := Encode(subject)
	m := &message{
		Code: msgCommit,
		Msg:  subjectPayload,
	}
	c.storeBacklog(m, p)
	c.processBacklog()

	const timeoutDura = 2 * time.Second
	timeout := time.NewTimer(timeoutDura)
	select {
	case e, ok := <-c.events.Chan():
		if !ok {
			return
		}
		t.Errorf("unexpected events comes: %v", e)
	case <-timeout.C:
		// success
	}
}

func TestProcessBacklog(t *testing.T) {
	v := &bft.View{
		Round:    big.NewInt(0),
		Sequence: big.NewInt(1),
	}
	preprepare := &bft.Preprepare{
		View:     v,
		Proposal: makeBlock(1),
	}
	prepreparePayload, _ := Encode(preprepare)

	subject := &bft.Subject{
		View:   v,
		Digest: common.StringToHash("1234567890"),
	}
	subjectPayload, _ := Encode(subject)

	msgs := []*message{
		&message{
			Code: msgPreprepare,
			Msg:  prepreparePayload,
		},
		&message{
			Code: msgPrepare,
			Msg:  subjectPayload,
		},
		&message{
			Code: msgCommit,
			Msg:  subjectPayload,
		},
		&message{
			Code: msgRoundChange,
			Msg:  subjectPayload,
		},
	}
	for i := 0; i < len(msgs); i++ {
		testProcessBacklog(t, msgs[i])
	}
}

func testProcessBacklog(t *testing.T, msg *message) {
	vset := newTestValidatorSet(1)
	backend := &testSystemBackend{
		events: new(event.TypeMux),
		peers:  vset,
	}
	c := &core{
		logger:     log.New("backend", "test", "id", 0),
		backlogs:   make(map[bft.Validator]*prque.Prque),
		backlogsMu: new(sync.Mutex),
		backend:    backend,
		state:      State(msg.Code),
		current: newRoundState(&bft.View{
			Sequence: big.NewInt(1),
			Round:    big.NewInt(0),
		}, newTestValidatorSet(4), common.Hash{}, nil, nil, nil),
	}
	c.subscribeEvents()
	defer c.unsubscribeEvents()

	c.storeBacklog(msg, vset.GetByIndex(0))
	c.processBacklog()

	const timeoutDura = 2 * time.Second
	timeout := time.NewTimer(timeoutDura)
	select {
	case ev := <-c.events.Chan():
		e, ok := ev.Data.(backlogEvent)
		if !ok {
			t.Errorf("unexpected event comes: %v", reflect.TypeOf(ev.Data))
		}
		if e.msg.Code != msg.Code {
			t.Errorf("message code mismatch: have %v, want %v", e.msg.Code, msg.Code)
		}
		// success
	case <-timeout.C:
		t.Error("unexpected timeout occurs")
	}
}
