package core

import (
	"math/big"
	"testing"

	"github.com/linkeye/linkeye/common"
	"github.com/linkeye/linkeye/consensus/dbft"
	"github.com/linkeye/linkeye/consensus/dbft/validator"
)

func TestRoundChangeSet(t *testing.T) {
	vset := validator.NewSet(generateValidators(4), bft.RoundRobin)
	rc := newRoundChangeSet(vset)

	view := &bft.View{
		Sequence: big.NewInt(1),
		Round:    big.NewInt(1),
	}
	r := &bft.Subject{
		View:   view,
		Digest: common.Hash{},
	}
	m, _ := Encode(r)

	// Test Add()
	// Add message from all validators
	for i, v := range vset.List() {
		msg := &message{
			Code:    msgRoundChange,
			Msg:     m,
			Address: v.Address(),
		}
		rc.Add(view.Round, msg)
		if rc.roundChanges[view.Round.Uint64()].Size() != i+1 {
			t.Errorf("the size of round change messages mismatch: have %v, want %v", rc.roundChanges[view.Round.Uint64()].Size(), i+1)
		}
	}

	// Add message again from all validators, but the size should be the same
	for _, v := range vset.List() {
		msg := &message{
			Code:    msgRoundChange,
			Msg:     m,
			Address: v.Address(),
		}
		rc.Add(view.Round, msg)
		if rc.roundChanges[view.Round.Uint64()].Size() != vset.Size() {
			t.Errorf("the size of round change messages mismatch: have %v, want %v", rc.roundChanges[view.Round.Uint64()].Size(), vset.Size())
		}
	}

	// Test MaxRound()
	for i := 0; i < 10; i++ {
		maxRound := rc.MaxRound(i)
		if i <= vset.Size() {
			if maxRound == nil || maxRound.Cmp(view.Round) != 0 {
				t.Errorf("max round mismatch: have %v, want %v", maxRound, view.Round)
			}
		} else if maxRound != nil {
			t.Errorf("max round mismatch: have %v, want nil", maxRound)
		}
	}

	// Test Clear()
	for i := int64(0); i < 2; i++ {
		rc.Clear(big.NewInt(i))
		if rc.roundChanges[view.Round.Uint64()].Size() != vset.Size() {
			t.Errorf("the size of round change messages mismatch: have %v, want %v", rc.roundChanges[view.Round.Uint64()].Size(), vset.Size())
		}
	}
	rc.Clear(big.NewInt(2))
	if rc.roundChanges[view.Round.Uint64()] != nil {
		t.Errorf("the change messages mismatch: have %v, want nil", rc.roundChanges[view.Round.Uint64()])
	}
}
