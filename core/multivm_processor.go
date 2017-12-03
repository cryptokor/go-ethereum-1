package core

import (
	"math/big"

	"github.com/ethereumproject/sputnikvm-ffi/go/sputnikvm"
	"github.com/ethereumproject/go-ethereum/common"
	"github.com/ethereumproject/go-ethereum/core/state"
	"github.com/ethereumproject/go-ethereum/core/types"
	evm "github.com/ethereumproject/go-ethereum/core/vm"
	"github.com/ethereumproject/go-ethereum/crypto"
	"github.com/ethereumproject/go-ethereum/logger"
	"github.com/ethereumproject/go-ethereum/logger/glog"
)

var (
	UseSputnikVM = false
)

func ApplyMultiVmTransaction(config *ChainConfig, bc *BlockChain, gp *GasPool, statedb *state.StateDB, header *types.Header, tx *types.Transaction, totalUsedGas *big.Int) (*types.Receipt, evm.Logs, *big.Int, error) {
	tx.SetSigner(config.GetSigner(header.Number))

	from, err := tx.From()
	if err != nil {
		return nil, nil, nil, err
	}
	vmtx := sputnikvm.Transaction {
		Caller: from,
		GasPrice: tx.GasPrice(),
		GasLimit: tx.Gas(),
		Address: tx.To(),
		Value: tx.Value(),
		Input: tx.Data(),
		Nonce: new(big.Int).SetUint64(tx.Nonce()),
	}
	vmheader := sputnikvm.HeaderParams {
		Beneficiary: header.Coinbase,
		Timestamp: header.Time.Uint64(),
		Number: header.Number,
		Difficulty: header.Difficulty,
		GasLimit: header.GasLimit,
	}

	current_number := header.Number
	homestead_fork := config.ForkByName("Homestead")
	eip150_fork := config.ForkByName("GasReprice")
	eip160_fork := config.ForkByName("Diehard")

	var vm *sputnikvm.VM
	if eip160_fork.Block != nil && current_number.Cmp(eip160_fork.Block) >= 0 {
		vm = sputnikvm.NewEIP160(&vmtx, &vmheader)
	} else if eip150_fork.Block != nil && current_number.Cmp(eip150_fork.Block) >= 0 {
		vm = sputnikvm.NewEIP150(&vmtx, &vmheader)
	} else if homestead_fork.Block != nil && current_number.Cmp(homestead_fork.Block) >= 0 {
		vm = sputnikvm.NewHomestead(&vmtx, &vmheader)
	} else {
		vm = sputnikvm.NewFrontier(&vmtx, &vmheader)
	}

Loop:
	for {
		ret := vm.Fire()
		switch ret.Typ() {
		case sputnikvm.RequireNone:
			break Loop
		case sputnikvm.RequireAccount:
			address := ret.Address()
			if statedb.Exist(address) {
				vm.CommitAccount(address, new(big.Int).SetUint64(statedb.GetNonce(address)),
					statedb.GetBalance(address), statedb.GetCode(address))
			} else {
				vm.CommitNonexist(address)
			}
		case sputnikvm.RequireAccountCode:
			address := ret.Address()
			if statedb.Exist(address) {
				vm.CommitAccountCode(address, statedb.GetCode(address))
			} else {
				vm.CommitNonexist(address)
			}
		case sputnikvm.RequireAccountStorage:
			address := ret.Address()
			key := common.BigToHash(ret.StorageKey())
			if statedb.Exist(address) {
				value := statedb.GetState(address, key).Big()
				key := ret.StorageKey()
				vm.CommitAccountStorage(address, key, value)
			} else {
				vm.CommitNonexist(address)
			}
		case sputnikvm.RequireBlockhash:
			number := ret.BlockNumber()
			hash := bc.GetBlockByNumber(number.Uint64()).Hash()
			vm.CommitBlockhash(number, hash)
		}
	}

	// VM execution is finished at this point. We apply changes to the statedb.

	for _, account := range vm.AccountChanges() {
		switch account.Typ() {
		case sputnikvm.AccountChangeIncreaseBalance:
			address := account.Address()
			amount := account.ChangedAmount()
			statedb.AddBalance(address, amount)
		case sputnikvm.AccountChangeDecreaseBalance:
			address := account.Address()
			amount := account.ChangedAmount()
			balance := new(big.Int).Sub(statedb.GetBalance(address), amount)
			statedb.SetBalance(address, balance)
		case sputnikvm.AccountChangeRemoved:
			address := account.Address()
			statedb.Suicide(address)
		case sputnikvm.AccountChangeFull:
			address := account.Address()
			code := account.Code()
			nonce := account.Nonce()
			balance := account.Balance()
			statedb.SetBalance(address, balance)
			statedb.SetNonce(address, nonce.Uint64())
			statedb.SetCode(address, code)
			for _, item := range account.ChangedStorage() {
				statedb.SetState(address, common.BigToHash(item.Key), common.BigToHash(item.Value))
			}
		case sputnikvm.AccountChangeCreate:
			address := account.Address()
			code := account.Code()
			nonce := account.Nonce()
			balance := account.Balance()
			statedb.SetBalance(address, balance)
			statedb.SetNonce(address, nonce.Uint64())
			statedb.SetCode(address, code)
			for _, item := range account.Storage() {
				statedb.SetState(address, common.BigToHash(item.Key), common.BigToHash(item.Value))
			}
		default:
			panic("unreachable")
		}
	}
	for _, log := range vm.Logs() {
		statelog := evm.NewLog(log.Address, log.Topics, log.Data, header.Number.Uint64())
		statedb.AddLog(statelog)
	}
	usedGas := vm.UsedGas()
	totalUsedGas.Add(totalUsedGas, usedGas)

	receipt := types.NewReceipt(statedb.IntermediateRoot().Bytes(), totalUsedGas)
	receipt.TxHash = tx.Hash()
	receipt.GasUsed = new(big.Int).Set(totalUsedGas)
	if MessageCreatesContract(tx) {
		from, _ := tx.From()
		receipt.ContractAddress = crypto.CreateAddress(from, tx.Nonce())
	}

	logs := statedb.GetLogs(tx.Hash())
	receipt.Logs = logs
	receipt.Bloom = types.CreateBloom(types.Receipts{receipt})

	glog.V(logger.Debug).Infoln(receipt)

	vm.Free()
	return receipt, logs, totalUsedGas, nil
}
