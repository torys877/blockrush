package internal

import (
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/olekukonko/tablewriter"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"
)

const (
	RpcCallPerSecond             = 600
	AttemptsToCollect            = 10
	AttemptsToCollectIntervalSec = 5
	LogsPath                     = "logs"
	DirPerm                      = 0755
	LogSuffix                    = "_output.log"
)

var (
	EmptyTests            = errors.New("no tests configured, please define at least one test")
	NotEnoughSenders      = errors.New("insufficient senders available, check sender configuration")
	CannotDecryptSenderPK = errors.New("failed to decrypt sender's private key, verify the provided key")
)

type Runner struct {
	config        Config
	client        *ethclient.Client
	tests         []Test
	senders       []*Sender
	metrics       []*Metrics
	totalTxsCount int
	errors        []error
}

func NewRunner(config Config, client *ethclient.Client) *Runner {
	return &Runner{
		config: config,
		client: client,
	}
}

func (r *Runner) Start() error {
	if len(r.config.Tests) == 0 {
		return EmptyTests
	}

	fmt.Println("Start Preparing data")
	handleErrors(&r.errors, r.PrepareSenders())
	handleErrors(&r.errors, r.PrepareTests())
	handleErrors(&r.errors, r.PrepareTransactions())

	fmt.Println("Tests Are Prepared")
	handleErrors(&r.errors, r.Run())
	fmt.Println("Txs Were Sent")

	fmt.Println("Begin Collect Metrics.")
	handleErrors(&r.errors, r.CollectData())
	handleErrors(&r.errors, r.CollectMetrics())
	r.Output()

	if len(r.errors) > 0 {
		fmt.Println("Errors occurred:")
		for _, err := range r.errors {
			fmt.Println(err)
		}
	}

	return nil
}

// PrepareSenders initializes sender entities from the provided private keys in the configuration.
func (r *Runner) PrepareSenders() error {
	// Iterate over private keys to initialize sender entities
	fmt.Println("Preparing Senders")
	for _, senderPK := range r.config.Senders.PrivateKeys {
		sender, err := NewSender(r.client, senderPK)
		if err != nil {
			return CannotDecryptSenderPK
		}

		err = sender.defineCurrentSenderNonce(sender)
		if err != nil {
			return fmt.Errorf("failed to set sender nonce: %w", err)
		}
		r.senders = append(r.senders, sender)
	}

	return nil
}

// PrepareTests initializes test cases from the configuration and validates sender availability.
func (r *Runner) PrepareTests() error {
	fmt.Println("Preparing Tests")
	r.totalTxsCount = 0
	for configName, configTest := range r.config.Tests {
		test := NewTest(r.client, r.config.App.Node.ChainID, configName, configTest)
		r.totalTxsCount += test.txsCount

		if configTest.Config.Senders > len(r.config.Senders.PrivateKeys) {
			return NotEnoughSenders
		}

		test.senders = r.senders[0:configTest.Config.Senders]

		r.tests = append(r.tests, *test)
	}

	return nil
}

// PrepareTransactions signs transactions for each test case, ensuring they are ready to be sent.
func (r *Runner) PrepareTransactions() error {
	fmt.Println("Prepare And Signing Transactions")
	for i := range r.tests {
		test := &r.tests[i]
		if r.tests[i].senderTransactions == nil {
			r.tests[i].senderTransactions = make(map[string][]*Transaction)
		}

		var err error
		if test.testType == SEND {
			err = test.SignTransactions()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *Runner) Run() error {
	if len(r.tests) == 0 {
		return EmptyTests
	}

	fmt.Println("Begin Sending Transactions")

	for testIdx := range r.tests {
		test := &r.tests[testIdx]
		err := test.Run()
		if err != nil {
			return err
		}
	}

	fmt.Println("Finish Sending Transactions")
	fmt.Println("Start Block: ", r.tests[0].startBlock)
	fmt.Println("End Block: ", r.tests[0].endBlock)

	return nil
}

// CollectData retrieves transaction data from the blockchain, tracking the status of sent transactions.
func (r *Runner) CollectData() error {
	time.Sleep(time.Duration(AttemptsToCollectIntervalSec) * time.Second)

	fmt.Println("Begin Collect Data")
	var totalCollectedTxCount int32

	for i := range r.tests {
		test := &r.tests[i]

		if test.testType == CALL {
			continue
		}

		go func() {
			for {
				fmt.Printf("Collected transactions: %d/%d\n", atomic.LoadInt32(&totalCollectedTxCount), r.totalTxsCount)
				time.Sleep(time.Second)
			}
		}()

		handleErrors(&r.errors, test.CollectData(&totalCollectedTxCount))
	}

	fmt.Println("End Collect Data")

	return nil
}

// CollectMetrics computes performance metrics such as TPS, gas usage, and transaction success rates.
func (r *Runner) CollectMetrics() error {
	for i := range r.tests {
		test := &r.tests[i]

		if test.testType == CALL {
			continue
		}

		handleErrors(&r.errors, test.CollectMetrics())

	}

	return nil
}

// Output renders the test results, including metrics and block data, in a human-readable table format.
func (r *Runner) Output() {
	for _, test := range r.tests {
		var file *os.File
		// prepare file for output metrics
		folderPath := filepath.Join(LogsPath, test.testName)
		fileName := test.testType + LogSuffix
		err := os.MkdirAll(folderPath, DirPerm)
		if err != nil {
			fmt.Printf("Could create folder for logs, error: %s, folderpath: %s\n", err.Error(), folderPath)
		} else {
			filePath := filepath.Join(folderPath, fileName)
			file, err = os.Create(filePath)
			if err != nil {
				log.Fatal(err)
			}
		}

		if test.testType == SEND {
			// CLI output
			r.outputSend(os.Stdout, test)

			// File output
			if file != nil {
				r.outputSend(file, test)
			}
		} else if test.testType == CALL {
			// CLI output
			r.outputCall(os.Stdout, test)

			// File output
			if file != nil {
				r.outputCall(file, test)
			}
		}
		if file != nil {
			handleErrors(&r.errors, file.Close())
		}
	}
}

func (r *Runner) outputSend(writer io.Writer, test Test) {
	data, dataBlocks := r.getSendOutputData(test)

	tableSummary := tablewriter.NewWriter(writer)
	tableBlocks := tablewriter.NewWriter(writer)

	tableSummary.SetHeader([]string{"Metric", "Result"})
	tableBlocks.SetHeader([]string{"", "Txs Count"})
	tableSummary.AppendBulk(data)
	tableBlocks.AppendBulk(dataBlocks)

	fmt.Fprint(writer, "\n\n============================================\n")
	fmt.Fprintf(writer, "Test (send): %s \n", test.testName)
	fmt.Fprint(writer, "============================================\n\n")
	tableSummary.Render()
	fmt.Fprintln(writer, "Block distance (average, distance between block when tx wax sent and block when tx was mined): ")
	tableBlocks.Render()
}

func (r *Runner) outputCall(writer io.Writer, test Test) {
	data := r.getCallOutputData(test)

	tableSummary := tablewriter.NewWriter(writer)
	tableBlocks := tablewriter.NewWriter(os.Stdout)

	tableSummary.SetHeader([]string{"Metric", "Result"})
	tableBlocks.SetHeader([]string{"", "Txs Count"})
	tableSummary.AppendBulk(data)

	fmt.Fprintln(writer, "============================================")
	fmt.Fprintf(writer, "Test (call): %s \n", test.testName)
	fmt.Fprintln(writer, "============================================")

	tableSummary.Render()
}

func (r *Runner) getSendOutputData(test Test) ([][]string, [][]string) {
	var data [][]string

	data = append(data, []string{"Senders", strconv.Itoa(len(test.senders))})
	data = append(data, []string{"Start Block", strconv.Itoa(int(test.startBlock))})
	data = append(data, []string{"End Block", strconv.Itoa(int(test.endBlock))})
	data = append(data, []string{"TPS (in config)", strconv.Itoa(int(test.metrics.configTps))})
	data = append(data, []string{"TXs In Block (avg)", strconv.Itoa(int(test.metrics.avgTxsPerBlock))})
	data = append(data, []string{"TXs Mine Time (avg, s)", strconv.FormatFloat(float64(test.metrics.avgTimeToInclude)/1000.0, 'f', 3, 64)})
	data = append(data, []string{"Gas Price per Tx (avg)", strconv.Itoa(int(test.metrics.avgGasPricePerTx))})
	data = append(data, []string{"Gas Usage per Block (avg)", strconv.Itoa(int(test.metrics.avgGasUsedPerBlock))})
	data = append(data, []string{"Success Txs", strconv.Itoa(int(test.metrics.succeedTxs))})
	data = append(data, []string{"Failed Txs", strconv.Itoa(int(test.metrics.failedTxs))})

	var i uint64 = 0
	processedBlocks := make(map[uint64]bool)
	var dataBlocks [][]string

	for len(processedBlocks) != len(test.metrics.avgTxsBlockDiffIncluded) {
		if _, exists := test.metrics.avgTxsBlockDiffIncluded[i]; exists {
			dataBlocks = append(dataBlocks, []string{fmt.Sprintf("+ %d", i), strconv.Itoa(int(test.metrics.avgTxsBlockDiffIncluded[i]))})
			processedBlocks[i] = true
		} else {
			dataBlocks = append(dataBlocks, []string{fmt.Sprintf("+ %d", i), "0"})
		}
		i++
	}

	return data, dataBlocks
}

func (r *Runner) getCallOutputData(test Test) [][]string {
	var data [][]string

	data = append(data, []string{"Senders", strconv.Itoa(len(test.senders))})
	data = append(data, []string{"Start Block", strconv.Itoa(int(test.startBlock))})
	data = append(data, []string{"End Block", strconv.Itoa(int(test.endBlock))})
	data = append(data, []string{"TPS", strconv.Itoa(int(test.callMetrics.configTps))})
	data = append(data, []string{"Total Sent Calls", strconv.Itoa(test.callMetrics.callSentCount)})
	data = append(data, []string{"Total Received Results", strconv.Itoa(test.callMetrics.callReceiveCount)})
	data = append(data, []string{"Total Error Calls", strconv.Itoa(test.callMetrics.callErrorsCount)})

	return data
}
