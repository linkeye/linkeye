package dbft

import "github.com/linkeye/linkeye/common"

type ProposerPolicy uint64

const (
	RoundRobin ProposerPolicy = iota
	Sticky
)

type Config struct {
	RequestTimeout uint64           `toml:",omitempty"` // The timeout for each DBFT round in milliseconds.
	BlockPeriod    uint64           `toml:",omitempty"` // Default minimum difference between two consecutive block's timestamps in second
	ProposerPolicy ProposerPolicy   `toml:",omitempty"` // The policy for proposer selection
	Epoch          uint64           `toml:",omitempty"` // The number of blocks after which to checkpoint and reset the pending votes
	Validators     []common.Address `toml:",omitempty"` // Genesis validator list
}

var DefaultConfig = &Config{
	RequestTimeout: 10000,
	BlockPeriod:    3,
	ProposerPolicy: RoundRobin,
	Epoch:          30000,
}
