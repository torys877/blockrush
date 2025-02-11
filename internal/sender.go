package internal

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

type Sender struct {
	client          *ethclient.Client
	Address         *common.Address
	PrivateKey      string
	PrivateKeyEcdsa *ecdsa.PrivateKey
	Nonce           uint64
}

func NewSender(client *ethclient.Client, senderPk string) (*Sender, error) {
	privateKey, err := crypto.HexToECDSA(senderPk)
	if err != nil {
		return nil, fmt.Errorf("failed to load private key: %w", err)
	}

	publicKey := privateKey.Public().(*ecdsa.PublicKey)
	fromAddress := crypto.PubkeyToAddress(*publicKey)

	return &Sender{
		client:          client,
		Address:         &fromAddress,
		PrivateKey:      senderPk,
		PrivateKeyEcdsa: privateKey,
	}, nil
}

func (s *Sender) defineCurrentSenderNonce(sender *Sender) error {
	nonce, err := s.client.PendingNonceAt(context.Background(), *sender.Address)
	if err != nil {
		return fmt.Errorf("failed to retrieve nonce for address %s: %w", sender.Address.Hex(), err)
	}

	sender.Nonce = nonce
	return nil
}
