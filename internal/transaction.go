package internal

import (
	"context"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"log"
	"math/big"
)

type Transaction struct {
	clientTransaction *types.Transaction
	receipt           *types.Receipt
	sender            string
	status            bool
	sentBlock         uint64
	minedBlock        uint64
	sentTimestamp     int64
}

func CreateAndSignTransaction(
	client *ethclient.Client,
	chainId int64,
	sender *Sender,
	receiver *common.Address,
	valueToSend *big.Int,
	data []byte,
) (*Transaction, error) {
	tipCap, err := client.SuggestGasTipCap(context.Background()) // maxPriorityFeePerGas
	if err != nil {
		log.Fatal("error getting tipCap:", err)
	}

	feeCap, err := client.SuggestGasPrice(context.Background()) // maxFeePerGas
	if err != nil {
		log.Fatal("error getting feeCap:", err)
	}

	dynTx := &types.DynamicFeeTx{
		Nonce:     sender.Nonce,
		To:        receiver,
		Value:     valueToSend,
		GasFeeCap: feeCap, // maxFeePerGas
		GasTipCap: tipCap, // maxPriorityFeePerGas
	}

	if len(data) != 0 {
		dynTx.Data = data
	}

	gasLimit, err := client.EstimateGas(context.Background(), ethereum.CallMsg{
		From: *sender.Address,
		To:   receiver,
		Data: data,
	})
	if err != nil {

		log.Fatalf("Failed to estimate gas: %v", err)
	}
	dynTx.Gas = gasLimit

	tx := types.NewTx(dynTx)

	signer := types.NewLondonSigner(big.NewInt(chainId))
	signedTx, err := types.SignTx(tx, signer, sender.PrivateKeyEcdsa)
	sender.Nonce = sender.Nonce + 1

	transaction := &Transaction{
		clientTransaction: signedTx,
		sender:            sender.Address.String(),
	}

	return transaction, nil
}
