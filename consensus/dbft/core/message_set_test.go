package core

import (
	"math/big"
	"testing"

	"github.com/linkeye/linkeye/common"
	"github.com/linkeye/linkeye/consensus/dbft"
	"github.com/linkeye/linkeye/rlp"
)

func TestMessageSetWithPreprepare(t *testing.T) {
	valSet := newTestValidatorSet(4)

	ms := newMessageSet(valSet)

	view := &bft.View{
		Round:    new(big.Int),
		Sequence: new(big.Int),
	}
	pp := &bft.Preprepare{
		View:     view,
		Proposal: makeBlock(1),
	}

	rawPP, err := rlp.EncodeToBytes(pp)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	msg := &message{
		Code:    msgPreprepare,
		Msg:     rawPP,
		Address: valSet.GetProposer().Address(),
	}

	err = ms.Add(msg)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	err = ms.Add(msg)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	if ms.Size() != 1 {
		t.Errorf("the size of message set mismatch: have %v, want 1", ms.Size())
	}
}

func TestMessageSetWithSubject(t *testing.T) {
	valSet := newTestValidatorSet(4)

	ms := newMessageSet(valSet)

	view := &bft.View{
		Round:    new(big.Int),
		Sequence: new(big.Int),
	}

	sub := &bft.Subject{
		View:   view,
		Digest: common.StringToHash("1234567890"),
	}

	rawSub, err := rlp.EncodeToBytes(sub)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	msg := &message{
		Code:    msgPrepare,
		Msg:     rawSub,
		Address: valSet.GetProposer().Address(),
	}

	err = ms.Add(msg)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	err = ms.Add(msg)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	if ms.Size() != 1 {
		t.Errorf("the size of message set mismatch: have %v, want 1", ms.Size())
	}
}
