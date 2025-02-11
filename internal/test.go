package internal

import (
	"context"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"log"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Test struct {
	client *ethclient.Client
	// config data
	chainId  int64
	testName string
	testType string
	dataSize int
	txsCount int
	duration int
	tps      int
	value    *big.Int
	// process data
	senders            []*Sender
	isContract         bool
	contract           Contract
	senderTransactions map[string][]*Transaction
	// metrics data
	blocks      []*types.Block
	startBlock  uint64
	endBlock    uint64
	metrics     *Metrics
	callMetrics *CallMetrics
}

type Contract struct {
	address      string
	functionData Function
}
type Function struct {
	name   string
	abi    string
	params []interface{} `yaml:"params"`
}

type Metrics struct {
	configTps               uint
	realTps                 uint
	avgTxsPerBlock          uint
	avgTxsBlockDiffIncluded map[uint64]uint // in which block tx were included after sent (block_tx_mined - block_tx_were_sent)
	avgTimeToInclude        uint            //in sec??
	avgGasPricePerTx        uint
	avgGasUsedPerBlock      uint
	succeedTxs              uint
	failedTxs               uint
}

type CallMetrics struct {
	configTps        uint
	callSentCount    int
	callReceiveCount int
	callErrorsCount  int
	errorMessages    []Message
}

type Message struct {
	Error   bool
	Message string
}

func NewTest(client *ethclient.Client, chainId int64, configTestName string, configTest TestEntity) *Test {
	test := &Test{
		client:   client,
		chainId:  chainId,
		testName: configTestName,
		testType: configTest.Type,
		txsCount: configTest.Config.TPS * configTest.Config.Duration,
		dataSize: configTest.Config.DataSize,
		duration: configTest.Config.Duration,
		tps:      configTest.Config.TPS,
		contract: Contract{
			address: configTest.Config.Contract.Address,
			functionData: Function{
				name:   configTest.Config.Contract.Function.Name,
				abi:    configTest.Config.Contract.Function.ABI,
				params: configTest.Config.Contract.Function.Params,
			},
		},
		isContract: configTest.Config.Contract.Address != "" &&
			configTest.Config.Contract.Function.Name != "" &&
			configTest.Config.Contract.Function.ABI != "",
	}

	if configTest.Config.value == "" {
		test.value = big.NewInt(0)
	} else {
		var err bool
		test.value, err = big.NewInt(0).SetString(configTest.Config.value, 10)
		if !err {
			fmt.Printf("failed to parse value for test '%s': using default value 0 \n", configTestName)
			test.value = big.NewInt(0)
		}
	}

	return test
}

func (t *Test) SignTransactions() error {
	txPerSender := t.txsCount / len(t.senders)
	var data []byte

	if t.isContract {
		parsedABI, err := abi.JSON(strings.NewReader(t.contract.functionData.abi))
		if err != nil {
			log.Fatalf("failed to parse contract ABI: %v", err)
		}

		var abiMethod abi.Method
		var exists bool
		if abiMethod, exists = parsedABI.Methods[t.contract.functionData.name]; !exists {
			return fmt.Errorf("invalid method name: %s (ensure the method exists in the contract ABI)", t.contract.functionData.name)
		}

		convertedParams, err := t.convertParams(abiMethod, t.contract.functionData.params)
		if err != nil {
			log.Fatalf("failed to convert function parameters: %v", err)
		}

		data, err = parsedABI.Pack(t.contract.functionData.name, convertedParams...)
		if err != nil {
			log.Fatalf("failed to pack ABI data: %v", err)
		}
	} else if t.dataSize != 0 {
		data = t.generateData(t.dataSize)
	}

	for _, sender := range t.senders {
		var receiver common.Address
		if t.isContract {
			receiver = common.HexToAddress(t.contract.address)
		} else {
			receiver = *sender.Address
		}

		for j := 0; j < txPerSender; j++ {
			signedTx, err := CreateAndSignTransaction(t.client, t.chainId, sender, &receiver, t.value, data)
			if err != nil {
				return fmt.Errorf("failed to sign transaction: %w", err)
			}

			t.senderTransactions[sender.Address.String()] =
				append(t.senderTransactions[sender.Address.String()], signedTx)
		}
	}

	return nil
}

func (t *Test) Run() error {
	fmt.Printf("Run Test: %s \n", t.testName)

	interval := time.Second / time.Duration(t.tps)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var wg sync.WaitGroup

	blockNumber, _ := t.client.BlockNumber(context.Background())
	t.startBlock = blockNumber
	for _, sender := range t.senders {
		wg.Add(1)
		if t.testType == SEND {
			go t.runSend(&wg, sender, ticker)
		} else if t.testType == CALL {
			callMsg, err := t.getCallMsg()

			if err != nil {
				fmt.Printf("Contract call message generation error: %s", err.Error())
				continue
			}

			t.callMetrics = &CallMetrics{
				configTps: uint(t.tps),
			}

			go t.runCall(&wg, callMsg, ticker)
		}
	}
	wg.Wait()

	blockNumber, _ = t.client.BlockNumber(context.Background())
	t.endBlock = blockNumber

	return nil
}

func (t *Test) runSend(wg *sync.WaitGroup, sender *Sender, ticker *time.Ticker) {
	defer wg.Done()
	senderAddress := sender.Address.String()
	for i, txSigned := range t.senderTransactions[senderAddress] {
		//get block before send TX
		blockNumber, _ := t.client.BlockNumber(context.Background())
		txSigned.sentBlock = blockNumber
		txSigned.sentTimestamp = time.Now().UnixMilli()
		t.senderTransactions[senderAddress][i] = txSigned

		// send TX to RPC
		err := t.client.SendTransaction(context.Background(), txSigned.clientTransaction)
		if err != nil {
			fmt.Printf("failed to send transaction: %v", err)
		}

		<-ticker.C
	}
}

func (t *Test) runCall(wg *sync.WaitGroup, callMsg *ethereum.CallMsg, ticker *time.Ticker) {
	defer wg.Done()
	for i := 0; i < t.txsCount; i++ {
		// call contract
		_, err := t.client.CallContract(context.Background(), *callMsg, nil)
		if err != nil {
			fmt.Printf("contract call failed: %v", err)
			t.callMetrics.callErrorsCount++
			t.callMetrics.errorMessages = append(t.callMetrics.errorMessages, Message{Error: true, Message: err.Error()})
		} else {
			t.callMetrics.callReceiveCount++
		}
		t.callMetrics.callSentCount++

		<-ticker.C
	}
}

func (t *Test) CollectData(totalCollectedTxCount *int32) error {
	// RpcCallPerSecond are common for test and divided by number of senders, each sender sends same amount of transactions
	interval := time.Second / time.Duration(RpcCallPerSecond)
	ticker := time.NewTicker(interval)
	var wg sync.WaitGroup
	var mu sync.Mutex
	blocks := make(map[string]bool)

	for _, sender := range t.senders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			senderAddress := sender.Address.String()
			totalTxCount := len(t.senderTransactions[senderAddress])
			collectedTxCount := 0
			txCollected := false
			attempts := 0
			for attempts < AttemptsToCollect && !txCollected {
				for txIndex := range t.senderTransactions[senderAddress] {
					txSent := t.senderTransactions[senderAddress][txIndex]
					if txSent.receipt != nil {
						continue
					}

					txReceipt, err := t.client.TransactionReceipt(context.Background(), txSent.clientTransaction.Hash())
					if err != nil {
						fmt.Printf("transaction not mined yet (attempt %d): txHash=%s \n", attempts+1, txSent.clientTransaction.Hash())
						continue
					}

					t.senderTransactions[senderAddress][txIndex].receipt = txReceipt
					atomic.AddInt32(totalCollectedTxCount, 1)

					mu.Lock()
					if _, exists := blocks[txReceipt.BlockHash.String()]; !exists {
						blocks[txReceipt.BlockHash.String()] = true
					}
					mu.Unlock()

					collectedTxCount++
					<-ticker.C
				}

				if collectedTxCount == totalTxCount {
					txCollected = true
				}
				attempts++
				time.Sleep(time.Second * time.Duration(AttemptsToCollectIntervalSec))
			}
		}()
	}
	wg.Wait()

	for blockHash := range blocks {
		block, err := t.client.BlockByHash(context.Background(), common.HexToHash(blockHash))
		if err != nil {
			log.Fatalf("failed to fetch block by hash: %v", err)
		}

		t.blocks = append(t.blocks, block)
	}

	sort.Slice(t.blocks, func(i, j int) bool {
		return t.blocks[i].Number().Cmp(t.blocks[j].Number()) < 0
	})

	return nil
}

func (t *Test) CollectMetrics() error {
	metrics := &Metrics{
		configTps:               uint(t.tps),
		succeedTxs:              0,
		failedTxs:               0,
		avgTxsBlockDiffIncluded: make(map[uint64]uint),
	}

	txCount := 0
	var gasUsed uint = 0
	for _, block := range t.blocks {
		txCount += block.Transactions().Len()
		gasUsed += uint(block.GasUsed())
	}

	if len(t.blocks) != 0 {
		metrics.avgTxsPerBlock = uint(txCount / len(t.blocks))
		metrics.avgGasUsedPerBlock = gasUsed / uint(len(t.blocks))
	}

	totalGasPrice := big.NewInt(0)
	var totalTxCount int64 = 0
	var totalTimeToInclude uint64 = 0

	for _, senderTxs := range t.senderTransactions {
		for _, tx := range senderTxs {
			metrics.avgTxsBlockDiffIncluded[tx.receipt.BlockNumber.Uint64()-tx.sentBlock]++
			// avgFeePerTx
			totalGasPrice = totalGasPrice.Add(totalGasPrice, tx.receipt.EffectiveGasPrice)
			// avgTimeToInclude
			for _, block := range t.blocks {
				if block.NumberU64() == tx.receipt.BlockNumber.Uint64() {
					timeDiff := time.Unix(int64(block.Time()), 0).Sub(time.UnixMilli(tx.sentTimestamp))
					totalTimeToInclude += uint64(timeDiff.Milliseconds())
				}
			}

			if tx.receipt.Status == 0 {
				metrics.failedTxs++
			} else {
				metrics.succeedTxs++
			}

			totalTxCount++
		}
	}

	if totalTxCount != 0 {
		metrics.avgTimeToInclude = uint(totalTimeToInclude / uint64(totalTxCount))
		metrics.avgGasPricePerTx = uint(totalGasPrice.Div(totalGasPrice, big.NewInt(totalTxCount)).Uint64())
	}

	t.metrics = metrics

	return nil
}

func (t *Test) getCallMsg() (*ethereum.CallMsg, error) {
	parsedABI, err := abi.JSON(strings.NewReader(t.contract.functionData.abi))
	if err != nil {
		log.Fatalf("failed to parse contract ABI (for callMsg): %v", err)
	}

	var abiMethod abi.Method
	var exists bool
	if abiMethod, exists = parsedABI.Methods[t.contract.functionData.name]; !exists {
		return nil, errors.New(fmt.Sprintf("Method Has Incorrect Name, name: %s \n", t.contract.functionData.name))
	}

	convertedParams, err := t.convertParams(abiMethod, t.contract.functionData.params)
	if err != nil {
		log.Fatalf("failed to convert function parameters: %v", err)
	}

	data, err := parsedABI.Pack(t.contract.functionData.name, convertedParams...)
	if err != nil {
		log.Fatalf("failed to pack ABI data: %v", err)
	}

	contractAddr := common.HexToAddress(t.contract.address)

	return &ethereum.CallMsg{
		To:   &contractAddr,
		Data: data,
	}, nil
}

func (t *Test) convertParams(method abi.Method, params []interface{}) ([]interface{}, error) {
	if len(method.Inputs) != len(params) {
		return nil, fmt.Errorf("parameter count mismatch: expected %d, got %d", len(method.Inputs), len(params))
	}

	convertedParams := make([]interface{}, len(params))

	for i, input := range method.Inputs {
		param := params[i]

		switch input.Type.T {
		case abi.AddressTy:
			if str, ok := param.(string); ok {
				convertedParams[i] = common.HexToAddress(str)
			} else {
				return nil, fmt.Errorf("parameter %d must be a valid Ethereum address (hex string)", i)
			}

		case abi.UintTy, abi.IntTy:
			if str, ok := param.(string); ok {
				value, success := new(big.Int).SetString(str, 10)
				if !success {
					return nil, fmt.Errorf("invalid uint256 value for parameter %d: %s", i, str)
				}
				convertedParams[i] = value
			} else {
				return nil, fmt.Errorf("parameter %d has an incorrect type or format", i)
			}

		case abi.BoolTy:
			if str, ok := param.(string); ok {
				value, err := strconv.ParseBool(str)
				if err != nil {
					return nil, fmt.Errorf("parameter %d must be a boolean (true/false or 'true'/'false')", i)
				}
				convertedParams[i] = value
			} else if b, ok := param.(bool); ok {
				convertedParams[i] = b
			} else {
				return nil, fmt.Errorf("parameter %d should be a bool", i)
			}

		case abi.StringTy:
			if str, ok := param.(string); ok {
				convertedParams[i] = str
			} else {
				return nil, fmt.Errorf("parameter %d must be a string", i)
			}

		default:
			return nil, fmt.Errorf("unsupported parameter type for parameter %d: %s", i, input.Type.String())
		}
	}

	return convertedParams, nil
}

// generateData create payload
func (t *Test) generateData(sizeInBytes int) []byte {
	data := make([]byte, sizeInBytes)

	for i := range data {
		data[i] = 0x41 // 'A'
	}

	return data
}
