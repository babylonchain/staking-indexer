# Staking Indexer

The staking indexer is a tool that extracts BTC staking relevant data from 
Bitcoin blockchain, transforms it into a structured form, loads it into a 
database, and pushes events to consumers. Data from the staking indexer is 
the ground truth for the BTC staking system.

## Features

* Polling BTC blocks data from a specified height in an ongoing manner. The 
  poller ensures that all the output blocks have at least `N` confirmations 
  where `N` is a configurable value.
* Extracting transaction data for staking, unbonding, and withdrawal.
  Specifications of identifying and parsing these transactions can be found 
  [here](./doc/extract_tx_data.md). 
* Storing the extracted transaction data in a database. The database schema 
  can be found [here](./doc/db_schema.md).
* Pushing staking, unbonding, and withdrawal events to the consumers.
  A reference implementation based on [rabbitmq](https://www.rabbitmq.com/) is 
  provided.
  The definition of each type of events can be found [here](./doc/events.md).
* Monitoring the status by Prometheus [metrics](./doc/metrics.md).

## Usage

### Setup bitcoind node

The staking indexer relies on `bitcoind` as backend. Follow this [guide](https://bitcoin.org/en/full-node#what-is-a-full-node)
to set up a `bitcoind` node.

### Install

Clone the repository to your local machine from Github:

```bash
git clone https://github.com/babylonchain/staking-indexer.git
```

Install the `sid` daemon binary by running:

```bash
cd staking-indexer # cd into the project directory
make install
```

### Configuration

To initiate the program with default config file, run:

```bash
sid init
```

This will create a `sid.conf` file in the default home directory. The 
default home directories for different operating systems are:

- **MacOS** `~/Users/<username>/Library/Application Support/Sid`
- **Linux** `~/.Sid`
- **Windows** `C:\Users\<username>\AppData\Local\Sid`

Use the `--home` flag to specify the home directory and use the `--force` to 
overwrite the existing config file.

### Run the Staking Indexer

To start the staking indexer from a specific height, run:

```bash
sid start --start-height <start-height>
```

The `--start-height` needs to be specified at the first run as the database 
does not have the last processed height. If `--start-height` is not 
specified, it will start from the last processed height retrieved from the 
database.

To run the staking indexer, we need to prepare a `global-params.json` file 
which defines all the global params that are used across the BTC staking 
system. The indexer needs it to parse staking 
transaction data. An example of the global params can be found in
[test-params.json](./itest/test-params.json).

The program reads the file from the home directory by default. The user can 
specify the file path using the `--params-path` flag.

### Tests

Run unit tests:

```bash
make test
```

Run e2e tests:

```bash
make test-e2e
```

This will initiate docker containers for both a `bitcoind` node running in the 
signet mode and a `rabbitmq` instance.
