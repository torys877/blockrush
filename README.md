# Blockrush: Blockchain Performance Testing Tool

## Overview
Blockrush is a CLI tool designed for blockchain and smart contracts performance testing, specifically targeting Ethereum networks (EVM compatible). It enables users to simulate and analyze blockchain transaction throughput, contract interactions, and network performance under various load conditions.

## Key Features
- 🚀 Multi-threaded Transaction Testing
- 📊 Performance Metrics Collection
- 🔄 Configurable Test Scenarios
- 📝 Logging and Reporting
- 🌐 Supports Multiple Blockchain Networks

## Requirements
- Go 1.23+
- Ethereum (EVM) Node (Local or Remote)
- Docker (Optional)

## Installation

### Local Setup
1. Clone the repository:
   ```bash
   git clone https://github.com/your-org/blockrush.git
   cd blockrush
   ```

2. Install dependencies:
   ```bash
   go mod tidy
   ```

3. Build the project:
   ```bash
   go build -o blockrush
   ```

### Docker Setup
```bash
docker build -t blockrush .
```

## Configuration
Blockrush uses a YAML configuration file to define test scenarios. An example configuration is provided in `config/config_example.yaml`.

### Configuration Structure
- `app.node`: Ethereum node connection settings
  - `rpc_url`: Node RPC endpoint
  - `chain_id`: Network identifier

- `tests`: Define multiple test scenarios with:
  - `type`: Transaction type (`send` or `call` - i.e. `eth_sendRawTransaction` or `eth_call`)
  - `config`:
    - `senders`: Number of concurrent test executors
    - `duration`: Test duration in seconds
    - `tps`: Target transactions per second
    - `data_size`: Transaction payload size (`optional`)
    - `value`: Ethers to send in WEI (`optional`)
    - `contract`: Contract interaction details (`optional`. If contract is defined, `data_size` will be ignored. All senders must have the necessary permissions to call the functions)
      - `address`: Contract address
      - `function`: Contract function data
        - `name`: Function name
        - `abi`: Function ABI
        - `params`: Function arguments (must be in the same order as in the smart contract)
- `senders`: Define test senders private keys (the number of private keys must be equal to or greater than senders number specified in the tests - `each test uses the same sender addresses`)

### Example Test Scenarios
1. **Simple Transaction Test**
   - Send transactions without contract interaction (ethers between accounts, can be with payload)
   - Configurable sender count and throughput

2. **Contract Transaction Test**
   - Execute specific contract function calls via sending transactions to contract
   - Test contract interaction performance

3. **Contract Call Test** (not fully implemented, todo: there should be collected more precise metrics)
    - Execute simple contract function calls (eg. `view`, `pure`)
    - Test contract interaction performance

## Running the Tool

### Local Execution
```bash
# Run with default configuration (NOTE: before run - replace `<YOUR_DEPLOYED_CONTRACT_ADDRESS>` in config/config_example.yaml with your contract address)  
./blockrush --config=config/config_example.yaml

# Customize configuration path
./blockrush --config=/path/to/custom/config.yaml
```

### Docker Execution
```bash
docker run --rm --network="host" -v $PWD/logs:/app/logs -v $PWD/config:/app/config -e CONFIG_PATH=/app/config/config_example.yaml blockrush
```

## Output and Metrics
Blockrush generates comprehensive performance reports including:
- Transaction success rates
- Transactions per second (TPS)
- Latency metrics
- Blocks metrics
- Gas consumption
- Error logs

Outputs are displayed in a human-readable table format and logged to `logs/` directory.


<details>
  <summary>Output Examples</summary>

```
Start Preparing data
Preparing Senders
Preparing Tests
Prepare And Signing Transactions
Tests Are Prepared
Begin Sending Transactions
Run Test: simple_transaction_test
Run Test: simple_transaction_test_nodata 
Run Test: contract_call_test 
Run Test: contract_send_test 
Finish Sending Transactions
Start Block:  95890
End Block:  95900
Txs Were Sent
Begin Collect Data
Collected transactions: 0/8400
Collected transactions: 497/8400
....
Collected transactions: 8190/8400
End Collect Data

============================================
Test (send): simple_transaction_test 
============================================

+---------------------------+------------+
|          METRIC           |   RESULT   |
+---------------------------+------------+
| Senders                   |          6 |
| Start Block               |      95890 |
| End Block                 |      95900 |
| TPS (in config)           |        400 |
| TXs In Block (avg)        |        384 |
| TXs Mine Time (avg, s)    |      1.446 |
| Gas Price per Tx (avg)    | 1000000007 |
| Gas Usage per Block (avg) |   55713586 |
| Success Txs               |       3996 |
| Failed Txs                |          0 |
+---------------------------+------------+
Block distance (average, distance between block when tx wax sent and block when tx was mined): 
+-----+-----------+
|     | TXS COUNT |
+-----+-----------+
| + 0 |         0 |
| + 1 |         0 |
| + 2 |      3987 |
| + 3 |         9 |
+-----+-----------+
```
</details>



## Advanced Features
- Multi-threaded transaction generation
- Support for both transaction sending and contract calls
- Flexible configuration for different test scenarios
- Automatic metrics collection and reporting
- Supports various Ethereum network configurations

## Logging
- Logs are stored in the `logs/` directory
- Each test run generates a log file

## Roadmap and TODOs
- [ ] Implement more advanced metrics collection
- [ ] Add detailed transaction and block logs
- [ ] Enhance private key management
- [ ] Add preliminary function calls for contracts before running tests (e.g., provide necessary access permissions for senders, if required)
- [ ] Add support for non-EVM blockchains


## License
This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Disclaimer
This tool is for testing and simulation purposes. Always use responsibly and in compliance with network guidelines.

