package internal

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	SEND = "send"
	CALL = "call"
)

// Config is a full config from file
type Config struct {
	App     AppConfig             `yaml:"app"`
	Tests   map[string]TestEntity `yaml:"tests"`
	Senders SendersConfig         `yaml:"senders"`
}

// AppConfig contains the main application settings.
type AppConfig struct {
	Node NodeConfig `yaml:"node"`
}

// NodeConfig holds configuration details for connecting to a blockchain node.
type NodeConfig struct {
	RPCURL  string `yaml:"rpc_url"`
	ChainID int64  `yaml:"chain_id"`
}

// TestEntity defines configuration parameters for test scenarios.
type TestEntity struct {
	Type   string     `yaml:"type"`
	Config TestConfig `yaml:"config"`
}

type TestConfig struct {
	Senders  int            `yaml:"senders"`
	Duration int            `yaml:"duration"`
	TPS      int            `yaml:"tps"`
	Contract ContractConfig `yaml:"contract"`
	DataSize int            `yaml:"data_size"`
	value    string         `yaml:"value"`
}

// ContractConfig contains configs for contract testing
type ContractConfig struct {
	Address  string         `yaml:"address"`
	Function FunctionConfig `yaml:"function"`
}

// FunctionConfig contains details about a smart contract function, including ABI and parameters.
// todo: add random function data
// todo: add function data from file
type FunctionConfig struct {
	Name   string        `yaml:"name"`
	ABI    string        `yaml:"abi"`
	Params []interface{} `yaml:"params"`
}

// SendersConfig stores sender-related configurations, including private keys.
// todo - add private keys from file and passphrase
type SendersConfig struct {
	PrivateKeys []string `yaml:"private_keys"`
}

// LoadConfig loads config yaml file in Config
func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file '%s': %w", filename, err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse configuration file '%s': %w", filename, err)
	}

	return &config, nil
}
