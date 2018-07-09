package core

import (
	"github.com/linkeye/linkeye/consensus/bft"
)

type backlogEvent struct {
	src bft.Validator
	msg *message
}

type timeoutEvent struct{}
