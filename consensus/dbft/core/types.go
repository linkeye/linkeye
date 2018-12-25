package core

import (
	"fmt"
	"io"

	"github.com/linkeye/linkeye/common"
	"github.com/linkeye/linkeye/rlp"
	"github.com/linkeye/linkeye/common/hexutil"
	bft "github.com/linkeye/linkeye/consensus/dbft"
)

type Engine interface {
	Start() error
	Stop() error
}

type State uint64

const (
	StateAcceptRequest State = iota
	StatePreprepared
	StatePrepared
	StateCommitted
)

func (s State) String() string {
	if s == StateAcceptRequest {
		return "Accept request"
	} else if s == StatePreprepared {
		return "Preprepared"
	} else if s == StatePrepared {
		return "Prepared"
	} else if s == StateCommitted {
		return "Committed"
	} else {
		return "Unknown"
	}
}

// Cmp compares s and y and returns:
//   -1 if s is the previous state of y
//    0 if s and y are the same state
//   +1 if s is the next state of y
func (s State) Cmp(y State) int {
	if uint64(s) < uint64(y) {
		return -1
	}
	if uint64(s) > uint64(y) {
		return 1
	}
	return 0
}

const (
	msgPreprepare uint64 = iota
	msgPrepare
	msgCommit
	msgRoundChange
	msgAll
)

type message struct {
	Code          uint64
	Msg           []byte
	Address       common.Address
	Signature     []byte
	CommittedSeal []byte
}

func (m *message) MsgStr() string {
	return hexutil.Encode(m.Msg)
}

func (m *message) SignatureStr() string {
	return hexutil.Encode(m.Signature)
}

func (m *message) CommittedSealStr() string {
	return hexutil.Encode(m.CommittedSeal)
}

// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes m into the Ethereum RLP format.
func (m *message) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{m.Code, m.Msg, m.Address, m.Signature, m.CommittedSeal})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (m *message) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		Code          uint64
		Msg           []byte
		Address       common.Address
		Signature     []byte
		CommittedSeal []byte
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	m.Code, m.Msg, m.Address, m.Signature, m.CommittedSeal = msg.Code, msg.Msg, msg.Address, msg.Signature, msg.CommittedSeal
	return nil
}

// ==============================================
//
// define the functions that needs to be provided for core.

func (m *message) FromPayload(b []byte, validateFn func([]byte, []byte) (common.Address, error)) error {
	// Decode message
	err := rlp.DecodeBytes(b, &m)
	if err != nil {
		return err
	}

	// Validate message (on a message without Signature)
	if validateFn != nil {
		var payload []byte
		payload, err = m.PayloadNoSig()
		if err != nil {
			return err
		}

		_, err = validateFn(payload, m.Signature)
	}
	// Still return the message even the err is not nil
	return err
}

func (m *message) Payload() ([]byte, error) {
	return rlp.EncodeToBytes(m)
}

func (m *message) PayloadNoSig() ([]byte, error) {
	return rlp.EncodeToBytes(&message{
		Code:          m.Code,
		Msg:           m.Msg,
		Address:       m.Address,
		Signature:     []byte{},
		CommittedSeal: m.CommittedSeal,
	})
}

func (m *message) Decode(val interface{}) error {
	return rlp.DecodeBytes(m.Msg, val)
}

func (m *message) String() string {
	// Decode PRE-PREPARE
	if m.Code == msgPreprepare {
		var preprepare *bft.Preprepare
		m.Decode(interface{}(&preprepare))
		return fmt.Sprintf("{Code: %v, Address: %v, preprepare.View: %v, Signature: %v, CommittedSeal: %v}", m.Code, m.Address.String(), preprepare.View, m.SignatureStr(), m.CommittedSealStr())
	} else if m.Code == msgPrepare {
		var subject *bft.Subject
		m.Decode(&subject)
		return fmt.Sprintf("{Code: %v, Address: %v, prepare: %v, Signature: %v, CommittedSeal: %v}", m.Code, m.Address.String(), subject, m.SignatureStr(), m.CommittedSealStr())
	} else if m.Code == msgCommit {
		var subject *bft.Subject
		m.Decode(&subject)
		return fmt.Sprintf("{Code: %v, Address: %v, commit: %v, Signature: %v, CommittedSeal: %v}", m.Code, m.Address.String(), subject, m.SignatureStr(), m.CommittedSealStr())
	} else if m.Code == msgRoundChange {
		var subject *bft.Subject
		m.Decode(&subject)
		return fmt.Sprintf("{Code: %v, Address: %v, roundChange: %v, Signature: %v, CommittedSeal: %v}", m.Code, m.Address.String(), subject, m.SignatureStr(), m.CommittedSealStr())
	} else if m.Code == msgAll  {

	}

	return fmt.Sprintf("{Code: %v, Address: %v, Msg: %v, Signature: %v, CommittedSeal: %v}", m.Code, m.Address.String(), m.Msg, m.SignatureStr(), m.CommittedSealStr())
}

// ==============================================
//
// helper functions

func Encode(val interface{}) ([]byte, error) {
	return rlp.EncodeToBytes(val)
}
