// Package evm defines the service to execute a step in an Ethereum Virtual
// Machine.
package evm

import (
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/store"
	"golang.org/x/xerrors"
)

var storeKey = [32]byte{0, 0, 10}
var gasUsageKey = [32]byte{0, 0, 20}
var runCountKey = [32]byte{0, 0, 30}
var resultKey = [32]byte{0, 0, 40}

var nilAddress = common.HexToAddress(
	"0x0000000000000000000000000000000000000000")

type evmService struct {
	stateDb      vm.StateDB
	contractAbi  abi.ABI
	instanceAddr common.Address
	accountAddr  common.Address
}

// GasLimit should be higher than the gas consumed during
// the smart contract execution. We chose 1e7 here because scalarMultBase takes
// around 500_000 units of gas
var txParams = struct {
	// maximum amount of Gas that a user is willing to pay for performing an
	// action or confirming a transaction
	GasLimit uint64

	// amount of Gwei (nano ether) that the user is willing to spend on each
	// unit of Gas
	GasPrice *big.Int
}{1e7, big.NewInt(1)}

var incrementMethod = "increment"
var scalarMultBaseMethod = "scalarMultBase"

// WeiPerEther ...
const WeiPerEther = 1e9

// EvmAccount is the abstraction for an Ethereum account
type EvmAccount struct {
	Address    common.Address
	PrivateKey *ecdsa.PrivateKey
	Nonce      uint64
}

// NewEvmAccount creates a new EvmAccount
func NewEvmAccount(privateKey string) (*EvmAccount, error) {
	privKey, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		return nil, xerrors.Errorf("failed to decode private "+
			"key for account creation: %v", err)
	}

	address := crypto.PubkeyToAddress(privKey.PublicKey)

	return &EvmAccount{
		Address:    address,
		PrivateKey: privKey,
	}, nil
}

// borrowed from cothority/bevm
var testPrivateKeys = []string{
	"c87509a1c067bbde78beb793e6fa76530b6382a4c0241e5e4a9ec0a0f44dc0d3",
	"ae6ae8e5ccbfb04590405997ee2d52d2b330726137b875053c36d94e974d162f",
	"8503d4206b83002eee8ffe8a11c2b09885a0912f5cddd2401d96c3abccca7401",
	"f78572bd69fbd3118ab756e3544d23821a2002b137c9037a3b8fd5b09169a73c",
}

// NewExection instantiates a new EvmService
func NewExecution(contract string) (*evmService, error) {
	account, err := NewEvmAccount(testPrivateKeys[0])
	if err != nil {
		return nil, xerrors.Errorf("failed to create EvmAccount: %v", err)
	}

	raw := rawdb.NewMemoryDatabase()

	db := state.NewDatabase(raw)

	var root common.Hash

	stateDb, err := state.New(root, db, nil)
	if err != nil {
		return nil, xerrors.Errorf("failed to create StateDB instance: %v", err)
	}

	root, err = stateDb.Commit(true)
	if err != nil {
		return nil, xerrors.Errorf("failed to commit state")
	}

	err = stateDb.Database().TrieDB().Commit(root, true, nil)
	if err != nil {
		return nil, xerrors.Errorf("failed to commit root to trie: %v", err)
	}

	contractJSON, err := ioutil.ReadFile(fmt.Sprintf("../evm/contracts/%s.abi", contract))
	if err != nil {
		return nil, xerrors.Errorf("failed to read %s contract abi: %v", contract, err)
	}

	contractAbi, err := abi.JSON(strings.NewReader(string(contractJSON)))
	if err != nil {
		return nil, xerrors.Errorf("failed to parse %s contract abi: %v", contract, err)
	}

	err = spawnContract(contract, contractAbi, stateDb, account)
	if err != nil {
		return nil, xerrors.Errorf("failed to spawn contract: %v", err)
	}

	instanceAddr := crypto.CreateAddress(account.Address, 0)

	return &evmService{
		contractAbi:  contractAbi,
		accountAddr:  account.Address,
		instanceAddr: instanceAddr,
		stateDb:      stateDb,
	}, nil
}

func spawnContract(contract string, contractAbi abi.ABI, stateDb *state.StateDB, account *EvmAccount) error {

	contractBuf, err := ioutil.ReadFile(fmt.Sprintf("../evm/contracts/%s.bin", contract))
	if err != nil {
		return xerrors.Errorf("failed to read %s contract: %v", contract, err)
	}

	contractHex := fmt.Sprintf("%s", contractBuf)

	contractByteCode, err := hex.DecodeString(strings.TrimSpace(contractHex))
	if err != nil {
		return xerrors.Errorf("failed to retrieve contract byte code: %v", err)
	}

	packedArgs, err := contractAbi.Pack("")
	if err != nil {
		return xerrors.Errorf("failed to pack args for %s contract: %v", contract, err)
	}

	callData := append(contractByteCode, packedArgs...)

	tx := types.NewContractCreation(uint64(0), big.NewInt(int64(0)), txParams.GasLimit, txParams.GasPrice, callData)

	var signer types.Signer = types.HomesteadSigner{}
	tx, err = types.SignTx(tx, signer, account.PrivateKey)
	if err != nil {
		return xerrors.Errorf("failed to sign transaction: %v", err)
	}

	balance, ok := new(big.Int).SetString("100000000000000000000", 10)
	if !ok {
		return xerrors.Errorf("failed to parse balance")
	}
	stateDb.AddBalance(account.Address, balance)

	// Gets the needed parameters
	chainConfig := getChainConfig()
	vmConfig := getVMConfig()

	// GasPool tracks the amount of gas available during execution of the
	// transactions in a block
	gp := new(core.GasPool).AddGas(uint64(1e15))
	usedGas := uint64(0)
	ug := &usedGas

	// ChainContext supports retrieving headers and consensus parameters from
	// the current blockchain to be used during transaction processing.
	var bc core.ChainContext

	timestamp := time.Now().UnixNano()

	// Header represents a block header in the Ethereum blockchain.
	header := &types.Header{
		Number:     big.NewInt(0),
		Difficulty: big.NewInt(0),
		ParentHash: common.Hash{0},
		Time:       uint64(timestamp),
	}

	receipt, err := core.ApplyTransaction(chainConfig, bc, &nilAddress, gp, stateDb, header, tx, ug, vmConfig)
	if err != nil {
		return xerrors.Errorf("failed to apply transaction: %v", err)
	}

	if receipt.Status != types.ReceiptStatusSuccessful {
		return xerrors.Errorf("failed to apply transaction: receipt status is not successful: %d", receipt.Status)
	}

	root, err := stateDb.Commit(true)
	if err != nil {
		return xerrors.Errorf("failed to commit state: %v", err)
	}

	err = stateDb.Database().TrieDB().Commit(root, true, nil)
	if err != nil {
		return xerrors.Errorf("failed to commit trie: %v", err)
	}

	//fmt.Printf("Used %d gas to spawn the contract\n", usedGas)

	return nil
}

func (e *evmService) Execute(snap store.Snapshot, step execution.Step) (execution.Result, error) {
	res := execution.Result{}

	current, err := snap.Get(storeKey[:])
	if err != nil {
		return res, xerrors.Errorf("failed to get store value: %v", err)
	}

	if len(current) == 0 {
		current = make([]byte, 32)
	}

	contract := string(step.Current.GetArg("contractName"))
	//fmt.Printf("Executing contract %s\n", contract)

	var output []byte
	if contract == "increment" {
		buf, err := e.ExecuteIncrement(current)
		if err != nil {
			return res, xerrors.Errorf("failed to execute increment contract: %v", err)
		}

		copy(output, buf)
	} else {
		currentCostBuf, err := snap.Get(gasUsageKey[:])
		if err != nil {
			return res, xerrors.Errorf("failed to get current cost: %v", err)
		}

		currentCost := binary.LittleEndian.Uint64(currentCostBuf)

		runCountBuf, err := snap.Get(runCountKey[:])
		if err != nil {
			return res, xerrors.Errorf("failed to get run count: %v", err)
		}

		runCount := binary.LittleEndian.Uint64(runCountBuf)

		buf, consumption, err := e.ExecuteEd25519(current)
		if err != nil {
			return res, xerrors.Errorf("failed to execute ed25519 contract: %v", err)
		}

		copy(output, buf)

		newCost := currentCost + consumption
		newCostBuf := make([]byte, 8)
		binary.LittleEndian.PutUint64(newCostBuf, newCost)
		snap.Set(gasUsageKey[:], newCostBuf)

		newRunCount := runCount + 1
		newRunCountBuf := make([]byte, 8)
		binary.LittleEndian.PutUint64(newRunCountBuf, newRunCount)
		snap.Set(runCountKey[:], newRunCountBuf)
	}

	snap.Set(resultKey[:], output)
	res.Accepted = true
	return res, nil
}

// ExecuteEd25519 executes the scalarMultBase method in the smart contract
// which generates an Ed25519 public key
func (e *evmService) ExecuteEd25519(input []byte) ([]byte, uint64, error) {
	contractAbi := e.contractAbi
	accountAddr := e.accountAddr
	instanceAddr := e.instanceAddr

	reverse(input) // input is in little-endian
	contractInput := new(big.Int).SetBytes(input)
	callData, err := contractAbi.Pack(scalarMultBaseMethod, contractInput)
	//fmt.Printf("input=%v\n", input)
	if err != nil {
		return nil, 0, xerrors.Errorf("failed to pack method `%s`: %v", "scalarMultBase", err)
	}

	timestamp := time.Now().UnixNano()

	evm := vm.NewEVM(getBlockContext(timestamp), getTxContext(), e.stateDb, getChainConfig(), getVMConfig())

	res, remainingGas, err := evm.Call(vm.AccountRef(accountAddr), instanceAddr, callData, uint64(1*WeiPerEther), big.NewInt(0))
	if err != nil {
		return nil, 0, xerrors.Errorf("failed to execute EVM view method: %+v", err)
	}

	methodAbi, ok := contractAbi.Methods[scalarMultBaseMethod]
	if !ok {
		return nil, 0, xerrors.Errorf("method `%s` does not exist for this contract: %v", scalarMultBaseMethod, err)
	}

	//outputBuf := make([]byte, 64)
	itfs, err := methodAbi.Outputs.UnpackValues(res)
	if err != nil {
		return nil, 0, xerrors.Errorf("failed to unpack values: %v", err)
	}

	if len(itfs) == 0 {
		return nil, 0, xerrors.Errorf("did not get output from the contract")
	}

	yInt := itfs[1].(*big.Int)
	y := yInt.Bytes()
	reverse(y)

	//y[31] ^= (x[0] & 1) << 7
	consumedGas := uint64(1*WeiPerEther) - remainingGas
	return y, consumedGas, nil
}

func reverse(buf []byte) {
	for i := 0; i < len(buf)/2; i++ {
		buf[i], buf[len(buf)-i-1] = buf[len(buf)-i-1], buf[i]
	}
}

// ExecuteEVM executes the `increment` contract wit the given input which
// represents a uint64 number in little-endian format. It returns `input+1`
// in little-endian format.
func (e *evmService) ExecuteIncrement(input []byte) ([]byte, error) {
	contractAbi := e.contractAbi
	accountAddr := e.accountAddr
	instanceAddr := e.instanceAddr

	currentVal := binary.LittleEndian.Uint64(input)

	callData, err := contractAbi.Pack(incrementMethod, new(big.Int).SetUint64(currentVal))
	if err != nil {
		return nil, xerrors.Errorf("failed to pack method `%s`: %v", incrementMethod, err)
	}

	timestamp := time.Now().UnixNano()

	evm := vm.NewEVM(getBlockContext(timestamp), getTxContext(), e.stateDb, getChainConfig(), getVMConfig())

	ret, _, err := evm.Call(vm.AccountRef(accountAddr), instanceAddr, callData, uint64(1*WeiPerEther), big.NewInt(0))
	if err != nil {
		return nil, xerrors.Errorf("failed to execute EVM view method: %v", err)
	}

	methodAbi, ok := contractAbi.Methods[incrementMethod]
	if !ok {
		return nil, xerrors.Errorf("method `%s` does not exist for this contract: %v", incrementMethod, err)
	}

	itfs, err := methodAbi.Outputs.UnpackValues(ret)
	if err != nil {
		return nil, xerrors.Errorf("failed to unpack values: %v", err)
	}

	if len(itfs) == 0 {
		return nil, xerrors.Errorf("did not get output from the contract")
	}

	val := itfs[0].(*big.Int)
	buf := val.Bytes()
	reverse(buf)

	for len(buf) < 8 {
		buf = append(buf, 0)
	}

	return buf, nil

}

func getBlockContext(timestamp int64) vm.BlockContext {
	placeHolder := common.HexToAddress("0")

	return vm.BlockContext{
		CanTransfer: func(vm.StateDB, common.Address, *big.Int) bool {
			return true
		},
		Transfer: func(vm.StateDB, common.Address, common.Address, *big.Int) {
		},
		GetHash: func(uint64) common.Hash {
			return common.HexToHash("0")
		},
		Coinbase:    placeHolder,
		GasLimit:    10000000000,
		BlockNumber: big.NewInt(0),
		Time:        big.NewInt(timestamp),
		Difficulty:  big.NewInt(1),
	}
}

func getTxContext() vm.TxContext {
	return vm.TxContext{
		Origin:   common.HexToAddress("0"),
		GasPrice: big.NewInt(0),
	}
}

func getChainConfig() *params.ChainConfig {
	// ChainConfig (adapted from Rinkeby test net)
	chainconfig := &params.ChainConfig{
		ChainID:        big.NewInt(1),
		HomesteadBlock: big.NewInt(0),
		DAOForkBlock:   nil,
		DAOForkSupport: false,
		EIP150Block:    big.NewInt(0), // required because Ed25519 contract has a staticall which consumes a lot of gas
		EIP150Hash: common.HexToHash(
			"0x0000000000000000000000000000000000000000"),
		EIP155Block:    big.NewInt(0),
		EIP158Block:    big.NewInt(0),
		ByzantiumBlock: big.NewInt(0),
		// Enable new Constantinople instructions
		ConstantinopleBlock: big.NewInt(0),
		Clique: &params.CliqueConfig{
			Period: 15,
			Epoch:  30000,
		},
	}

	return chainconfig
}

func getVMConfig() vm.Config {
	// vmConfig Config
	vmconfig := &vm.Config{
		// Debug enables debugging Interpreter options
		Debug: false,
		// Tracer is the op code logger
		Tracer: vm.NewStructLogger(&vm.LogConfig{
			Debug: true,
		}),
		// NoRecursion disables Interpreter call, callcode,
		// delegate call and create.
		NoRecursion: false,
		// Enable recording of SHA3/keccak preimages
		EnablePreimageRecording: true,
		// JumpTable contains the EVM instruction table. This
		// may be left uninitialised and will be set to the default
		// table.
		//JumpTable [256]operation
		//JumpTable: ,
		// Type of the EWASM interpreter
		EWASMInterpreter: "",
		// Type of the EVM interpreter
		EVMInterpreter: "",
	}

	return *vmconfig
}
