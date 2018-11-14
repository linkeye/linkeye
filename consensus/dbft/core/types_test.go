package core

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/linkeye/linkeye/common"
	"github.com/linkeye/linkeye/consensus/dbft"
)

func testPreprepare(t *testing.T) {
	pp := &bft.Preprepare{
		View: &bft.View{
			Round:    big.NewInt(1),
			Sequence: big.NewInt(2),
		},
		Proposal: makeBlock(1),
	}
	prepreparePayload, _ := Encode(pp)

	m := &message{
		Code:    msgPreprepare,
		Msg:     prepreparePayload,
		Address: common.HexToAddress("0x1234567890"),
	}

	msgPayload, err := m.Payload()
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	decodedMsg := new(message)
	err = decodedMsg.FromPayload(msgPayload, nil)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	var decodedPP *bft.Preprepare
	err = decodedMsg.Decode(&decodedPP)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	// if block is encoded/decoded by rlp, we cannot to compare interface data type using reflect.DeepEqual. (like bft.Proposal)
	// so individual comparison here.
	if !reflect.DeepEqual(pp.Proposal.Hash(), decodedPP.Proposal.Hash()) {
		t.Errorf("proposal hash mismatch: have %v, want %v", decodedPP.Proposal.Hash(), pp.Proposal.Hash())
	}

	if !reflect.DeepEqual(pp.View, decodedPP.View) {
		t.Errorf("view mismatch: have %v, want %v", decodedPP.View, pp.View)
	}

	if !reflect.DeepEqual(pp.Proposal.Number(), decodedPP.Proposal.Number()) {
		t.Errorf("proposal number mismatch: have %v, want %v", decodedPP.Proposal.Number(), pp.Proposal.Number())
	}
}

func testSubject(t *testing.T) {
	s := &bft.Subject{
		View: &bft.View{
			Round:    big.NewInt(1),
			Sequence: big.NewInt(2),
		},
		Digest: common.StringToHash("1234567890"),
	}

	subjectPayload, _ := Encode(s)

	m := &message{
		Code:    msgPreprepare,
		Msg:     subjectPayload,
		Address: common.HexToAddress("0x1234567890"),
	}

	msgPayload, err := m.Payload()
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	decodedMsg := new(message)
	err = decodedMsg.FromPayload(msgPayload, nil)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	var decodedSub *bft.Subject
	err = decodedMsg.Decode(&decodedSub)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	if !reflect.DeepEqual(s, decodedSub) {
		t.Errorf("subject mismatch: have %v, want %v", decodedSub, s)
	}
}

func testSubjectWithSignature(t *testing.T) {
	s := &bft.Subject{
		View: &bft.View{
			Round:    big.NewInt(1),
			Sequence: big.NewInt(2),
		},
		Digest: common.StringToHash("1234567890"),
	}
	expectedSig := []byte{0x01}

	subjectPayload, _ := Encode(s)
	// 1. Encode test
	m := &message{
		Code:          msgPreprepare,
		Msg:           subjectPayload,
		Address:       common.HexToAddress("0x1234567890"),
		Signature:     expectedSig,
		CommittedSeal: []byte{},
	}

	msgPayload, err := m.Payload()
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	// 2. Decode test
	// 2.1 Test normal validate func
	decodedMsg := new(message)
	err = decodedMsg.FromPayload(msgPayload, func(data []byte, sig []byte) (common.Address, error) {
		return common.Address{}, nil
	})
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	if !reflect.DeepEqual(decodedMsg, m) {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	// 2.2 Test nil validate func
	decodedMsg = new(message)
	err = decodedMsg.FromPayload(msgPayload, nil)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(decodedMsg, m) {
		t.Errorf("message mismatch: have %v, want %v", decodedMsg, m)
	}

	// 2.3 Test failed validate func
	decodedMsg = new(message)
	err = decodedMsg.FromPayload(msgPayload, func(data []byte, sig []byte) (common.Address, error) {
		return common.Address{}, bft.ErrUnauthorizedAddress
	})
	if err != bft.ErrUnauthorizedAddress {
		t.Errorf("error mismatch: have %v, want %v", err, bft.ErrUnauthorizedAddress)
	}
}

func TestMessageEncodeDecode(t *testing.T) {
	testPreprepare(t)
	testSubject(t)
	testSubjectWithSignature(t)
}
