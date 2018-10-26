package core

import (
	"bytes"
	"math/big"

	"github.com/linkeye/linkeye/common"
	"github.com/linkeye/linkeye/consensus"
	"github.com/linkeye/linkeye/consensus/dpos"
	"github.com/linkeye/linkeye/consensus/misc"
	"github.com/linkeye/linkeye/core/state"
	"github.com/linkeye/linkeye/core/types"
	"github.com/linkeye/linkeye/core/vm"
	"github.com/linkeye/linkeye/crypto"
	"github.com/linkeye/linkeye/log"
	"github.com/linkeye/linkeye/params"
)

// StateProcessor is a basic Processor, which takes care of transitioning
// state from one point to another.
//
// StateProcessor implements Processor.
type StateProcessor struct {
	config *params.ChainConfig // Chain configuration options
	bc     *BlockChain         // Canonical block chain
	engine consensus.Engine    // Consensus engine used for block rewards
}

// NewStateProcessor initialises a new StateProcessor.
func NewStateProcessor(config *params.ChainConfig, bc *BlockChain, engine consensus.Engine) *StateProcessor {
	return &StateProcessor{
		config: config,
		bc:     bc,
		engine: engine,
	}
}

// Process processes the state changes according to the Ethereum rules by running
// the transaction messages using the statedb and applying any rewards to both
// the processor (coinbase) and any included uncles.
//
// Process returns the receipts and logs accumulated during the process and
// returns the amount of gas that was used in the process. If any of the
// transactions failed to execute due to insufficient gas it will return an error.
func (p *StateProcessor) Process(block *types.Block, statedb *state.StateDB, cfg vm.Config) (types.Receipts, []*types.Log, uint64, error) {
	var (
		receipts types.Receipts
		usedGas  = new(uint64)
		header   = block.Header()
		allLogs  []*types.Log
		gp       = new(GasPool).AddGas(block.GasLimit())
	)
	// Mutate the the block and state according to any hard-fork specs
	if p.config.DAOForkSupport && p.config.DAOForkBlock != nil && p.config.DAOForkBlock.Cmp(block.Number()) == 0 {
		misc.ApplyDAOHardFork(statedb)
	}
	// Iterate over and process the individual transactions
	for i, tx := range block.Transactions() {
		statedb.Prepare(tx.Hash(), block.Hash(), i)
		receipt, _, err := ApplyTransaction(p.config, block.DposCtx(), p.bc, nil, gp, statedb, header, tx, usedGas, cfg)
		if err != nil {
			return nil, nil, 0, err
		}
		receipts = append(receipts, receipt)
		allLogs = append(allLogs, receipt.Logs...)
	}
	// Finalize the block, applying any consensus engine specific extras (e.g. block rewards)
	p.engine.Finalize(p.bc, header, statedb, block.Transactions(), block.Uncles(), receipts, block.DposCtx())

	return receipts, allLogs, *usedGas, nil
}

// ApplyTransaction attempts to apply a transaction to the given state database
// and uses the input parameters for its environment. It returns the receiptValidateDposState
// for the transaction, gas used and an error if the transaction failed,ValidateDposState
// indicating the block was invalid.ValidateDposState
func ApplyTransaction(config *params.ChainConfig, dposContext *types.DposContext, bc *BlockChain, author *common.Address, gp *GasPool, statedb *state.StateDB, header *types.Header, tx *types.Transaction, usedGas *uint64, cfg vm.Config) (*types.Receipt, uint64, error) {
	msg, err := tx.AsMessage(types.MakeSigner(config, header.Number))
	if err != nil {
		return nil, 0, err
	}

	if msg.To() == nil && msg.Type() != types.Binary {
		return nil, 0, types.ErrInvalidType
	}

	// Create a new context to be used in the EVM environment
	context := NewEVMContext(msg, header, bc, author)
	// Create a new environment which holds all relevant information
	// about the transaction and calling mechanisms.
	vmenv := vm.NewEVM(context, statedb, config, cfg)
	// Apply the transaction to the current state (included in the env)
	_, gas, failed, err := ApplyMessage(vmenv, msg, gp)
	if err != nil {
		log.Error("ApplyTransaction:", "err", err)
		return nil, 0, err
	}
	if msg.Type() != types.Binary {
		if err = applyDposMessage(dposContext, msg, statedb, header); err != nil {
			log.Error("applyDposMsg:", "err", err)
			return nil, 0, err
		}
	}

	// Update the state with pending changes
	var root []byte
	if config.IsByzantium(header.Number) {
		statedb.Finalise(true)
	} else {
		root = statedb.IntermediateRoot(config.IsEIP158(header.Number)).Bytes()
	}
	*usedGas += gas

	// Create a new receipt for the transaction, storing the intermediate root and gas used by the tx
	// based on the eip phase, we're passing wether the root touch-delete accounts.
	receipt := types.NewReceipt(root, failed, *usedGas)
	receipt.TxHash = tx.Hash()
	receipt.GasUsed = gas
	// if the transaction created a contract, store the creation address in the receipt.
	if msg.To() == nil {
		receipt.ContractAddress = crypto.CreateAddress(vmenv.Context.Origin, tx.Nonce())
	}
	// Set the receipt logs and create a bloom for filtering
	receipt.Logs = statedb.GetLogs(tx.Hash())
	receipt.Bloom = types.CreateBloom(types.Receipts{receipt})

	return receipt, gas, err
}

func addDposLog(statedb *state.StateDB, addr common.Address, topics, data string, header *types.Header) {
	hash := crypto.Keccak256Hash([]byte(topics))
	statedb.AddLog(&types.Log{
		Address:     addr,
		Topics:      []common.Hash{hash},
		Data:        []byte(data),
		BlockNumber: header.Number.Uint64(),
	})
}

func applyDposMessage(dposContext *types.DposContext, msg types.Message, statedb *state.StateDB, header *types.Header) error {
	switch msg.Type() {
	case types.LoginCandidate:
		log.Info("LoginCandidate:")
		if cc, err := dposContext.GetCandidateContext(msg.From()); err != nil {
			log.Error("failed to GetCandidateContext:", "from", msg.From())
			addDposLog(statedb, msg.From(), "LoginCandidate", "failed to GetCandidateContext"+msg.From().String(), header)

			return nil
		} else {
			// when state is logout, then login, change state to login
			if cc.State == types.LogoutState {
				cc := &types.CandidateContext{
					Addr:        msg.From(),
					State:       types.LoginState,
					BlockNumber: header.Number,
					Score:       cc.Score,
				}
				dposContext.SetCandidateContext(*cc)
			} else if cc.State == types.LoginState && bytes.Compare(cc.Addr.Bytes(), msg.From().Bytes()) == 0 {
				addDposLog(statedb, msg.From(), "RepeatLoginCandidate", "You are already in login state before, "+msg.From().String(), header)
				return nil
			} else {
				// first login
				log.Info("applyDposMessage:", "LoginCandidate, from", msg.From())
				bal := statedb.GetBalance(msg.From())
				f := new(big.Float).SetInt(bal)
				if f.Cmp(dpos.MinCandidateBalance) < 0 {
					log.Error("insufficient balance to become candidate")
					addDposLog(statedb, msg.From(), "InsufficientBalance", "insufficient balance to become candidate, "+msg.From().String(), header)
					return nil
				}
				minCandidate := new(big.Int)
				dpos.MinCandidateBalance.Int(minCandidate)
				log.Info("LoginCandidate", "minCandidate", minCandidate)
				statedb.SubBalance(msg.From(), minCandidate)
				cc := &types.CandidateContext{
					Addr:        msg.From(),
					State:       types.LoginState, //扣币抵押
					BlockNumber: header.Number,    //记录成为候选人时的区块高度
					Score:       big.NewInt(0),
				}
				dposContext.SetCandidateContext(*cc)
			}
		}
	case types.LogoutCandidate:
		log.Info("LogoutCandidate:")
		if cc, err := dposContext.GetCandidateContext(msg.From()); err != nil {
			log.Error("failed to GetCandidateContext:", "from", msg.From())
			addDposLog(statedb, msg.From(), "LogoutCandidate", "failed to GetCandidateContext"+msg.From().String(), header)
			return nil
		} else {
			//只在币处于login的状态下，才允许调用logout
			if cc.State == types.LoginState && bytes.Compare(cc.Addr.Bytes(), msg.From().Bytes()) == 0 {
				//更新候选人币的对应状态，更新为解锁状态中,记录开始解锁时的区块高度
				cc := &types.CandidateContext{
					Addr:        msg.From(),
					State:       types.LogoutState,
					BlockNumber: header.Number,
					Score:       cc.Score,
				}
				dposContext.SetCandidateContext(*cc)
			} else if cc.State == types.LogoutState {
				addDposLog(statedb, msg.From(), "RepeatLogoutCandidate", "You are already in logout state before, "+msg.From().String(), header)
				return nil
			} else {
				addDposLog(statedb, msg.From(), "NoLogin", "please first call login, "+msg.From().String(), header)
				return nil
			}
		}
	case types.WithdrawCandidate:
		if cc, err := dposContext.GetCandidateContext(msg.From()); err != nil {
			log.Error("failed to GetCandidateContext:", "from", msg.From())
			addDposLog(statedb, msg.From(), "WithdrawCandidate", "failed to GetCandidateContext"+msg.From().String(), header)
			return nil
		} else {
			//只在币处于logout的状态下，才允许withdraw
			if cc.State == types.LogoutState {
				log.Info("compare blocktime:")
				//解锁时间已到
				if new(big.Int).Add(cc.BlockNumber, dpos.UnlockInterval).Cmp(header.Number) < 0 {
					if err := dposContext.KickoutCandidate(msg.From()); err != nil {
						addDposLog(statedb, msg.From(), "KickoutCandidate", "failed to KickoutCandidate "+msg.From().String(), header)
						return nil
					}
					minCandidate := new(big.Int)
					minCandidate, _ = dpos.MinCandidateBalance.Int(minCandidate)
					log.Info("add balance:", "from", msg.From(), "minCandidate", minCandidate)
					statedb.AddBalance(msg.From(), minCandidate) //退出成为候选人时，退回抵押的币
				} else {
					addDposLog(statedb, msg.From(), "Locking", "please wait, unlock time isn't arrive, "+msg.From().String(), header)
					return nil
				}
			} else {
				addDposLog(statedb, msg.From(), "NoLogout", "please first call logout, "+msg.From().String(), header)
				return nil
			}
		}
	case types.Delegate:
		dposContext.Delegate(msg.From(), *(msg.To()))
	case types.UnDelegate:
		dposContext.UnDelegate(msg.From(), *(msg.To()))
	default:
		return types.ErrInvalidType
	}
	return nil
}
