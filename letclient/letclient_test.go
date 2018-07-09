package letclient

import "github.com/linkeye/linkeye"

// Verify that Client implements the linkeye interfaces.
var (
	_ = linkeye.ChainReader(&Client{})
	_ = linkeye.TransactionReader(&Client{})
	_ = linkeye.ChainStateReader(&Client{})
	_ = linkeye.ChainSyncReader(&Client{})
	_ = linkeye.ContractCaller(&Client{})
	_ = linkeye.GasEstimator(&Client{})
	_ = linkeye.GasPricer(&Client{})
	_ = linkeye.LogFilterer(&Client{})
	_ = linkeye.PendingStateReader(&Client{})
	// _ = linkeye.PendingStateEventer(&Client{})
	_ = linkeye.PendingContractCaller(&Client{})
)
