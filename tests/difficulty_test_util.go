package tests

import (
	"math/big"

	"github.com/linkeye/linkeye/common"
	"github.com/linkeye/linkeye/common/math"
)

//go:generate gencodec -type DifficultyTest -field-override difficultyTestMarshaling -out gen_difficultytest.go

type DifficultyTest struct {
	ParentTimestamp    *big.Int    `json:"parentTimestamp"`
	ParentDifficulty   *big.Int    `json:"parentDifficulty"`
	UncleHash          common.Hash `json:"parentUncles"`
	CurrentTimestamp   *big.Int    `json:"currentTimestamp"`
	CurrentBlockNumber uint64      `json:"currentBlockNumber"`
	CurrentDifficulty  *big.Int    `json:"currentDifficulty"`
}

type difficultyTestMarshaling struct {
	ParentTimestamp    *math.HexOrDecimal256
	ParentDifficulty   *math.HexOrDecimal256
	CurrentTimestamp   *math.HexOrDecimal256
	CurrentDifficulty  *math.HexOrDecimal256
	UncleHash          common.Hash
	CurrentBlockNumber math.HexOrDecimal64
}

// func (test *DifficultyTest) Run(config *params.ChainConfig) error {
// 	parentNumber := big.NewInt(int64(test.CurrentBlockNumber - 1))
// 	parent := &types.Header{
// 		Difficulty: test.ParentDifficulty,
// 		Time:       test.ParentTimestamp,
// 		Number:     parentNumber,
// 		UncleHash:  test.UncleHash,
// 	}

// 	actual := lethash.CalcDifficulty(config, test.CurrentTimestamp.Uint64(), parent)
// 	exp := test.CurrentDifficulty

// 	if actual.Cmp(exp) != 0 {
// 		return fmt.Errorf("parent[time %v diff %v unclehash:%x] child[time %v number %v] diff %v != expected %v",
// 			test.ParentTimestamp, test.ParentDifficulty, test.UncleHash,
// 			test.CurrentTimestamp, test.CurrentBlockNumber, actual, exp)
// 	}
// 	return nil

// }
