package main

import (
	"blockrush/internal"
	"log"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"
)

var configFile string
var config *internal.Config

var rootCmd = &cobra.Command{
	Use:   "blockrush",
	Short: "Blockrush CLI tool",
	Run: func(cmd *cobra.Command, args []string) {
		var err error
		config, err = internal.LoadConfig(configFile)
		if err != nil {
			log.Fatalf("Error loading configuration file: %v", err)
		}
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "path to config file")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatalf("Command execution failed: %v", err)
	}

	client, err := ethclient.Dial(config.App.Node.RPCURL)
	if err != nil {
		log.Fatalf("Unable to establish connection with Ethereum node: %v", err)
	}

	runner := internal.NewRunner(*config, client)
	err = runner.Start()
	if err != nil {
		log.Fatalf("Runner encountered an error: %v", err)
	}
}
