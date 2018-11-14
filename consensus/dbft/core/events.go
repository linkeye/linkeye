package core

import (
	bft "github.com/linkeye/linkeye/consensus/dbft"
)

type backlogEvent struct {
	src bft.Validator
	msg *message
}

type timeoutEvent struct{}
