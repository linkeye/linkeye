package dbft

import "errors"

var (
	// ErrUnauthorizedAddress is returned when given address cannot be found in
	// current validator set.
	ErrUnauthorizedAddress = errors.New("unauthorized address")
	// ErrStoppedEngine is returned if the engine is stopped
	ErrStoppedEngine = errors.New("stopped engine")
	// ErrStartedEngine is returned if the engine is already started
	ErrStartedEngine = errors.New("started engine")
	// ErrMsg is returned if the msg is not good
	ErrMsg = error.New("bad message")
)
