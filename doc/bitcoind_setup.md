# Setup Bitcoin Node

The `stakerd` daemon requires a running Bitcoin node and a **legacy** wallet loaded
with signet Bitcoins to perform staking operations.

You can configure `stakerd` daemon to connect to either
`bitcoind` or `btcd` node types. While both are compatible, we recommend
using `bitcoind`. Ensure that you are using legacy wallets, as `stakerd` daemon
doesn't currently support descriptor wallets.

Below, we'll guide you through setting up a signet `bitcoind` node and a legacy
wallet:

### 2.1. Download and Extract Bitcoin Binary:

```bash
# Download Bitcoin Core binary
wget https://bitcoincore.org/bin/bitcoin-core-26.0/bitcoin-26.0-x86_64-linux-gnu.tar.gz

# Extract the downloaded archive
tar -xvf bitcoin-26.0-x86_64-linux-gnu.tar.gz

# Provide execution permissions to binaries
chmod +x bitcoin-26.0/bin/bitcoind
chmod +x bitcoin-26.0/bin/bitcoin-cli
```

### 2.2. Create and start a Systemd Service:

Please update the following configurations in the provided file:

1. Replace `<your_rpc_username>` and `<your_rpc_password>` with your own values.
   These credentials will also be utilized in the btc-staker configuration file later
   on.
2. Ensure that the `<user>` is set to the machine user. In the guide below, it's set
   to `ubuntu`.
3. Note that `deprecatedrpc=create_bdb` is necessary to enable the creation of a
   legacy wallet, which has been deprecated in the latest core version. For more
   information, refer to the Bitcoin Core 26.0 release
   page [here](https://bitcoincore.org/en/releases/26.0/)
   and this [link](https://github.com/bitcoin/bitcoin/pull/28597).
4. If you want to enable remote connections to the node, you can add
   `rpcallowip=0.0.0.0/0` and `rpcbind=0.0.0.0` to the bitcoind command.

```bash 
# Create the service file
sudo tee /etc/systemd/system/bitcoind.service >/dev/null <<EOF
[Unit]
Description=bitcoin signet node
After=network.target

[Service]
User=<user>
Type=simple
ExecStart=/home/ubuntu/bitcoin-26.0/bin/bitcoind \
    -deprecatedrpc=create_bdb \
    -signet \
    -server \
    -rpcport=38332 \
    -rpcuser=<your_rpc_username> \
    -rpcpassword=<your_rpc_password>
Restart=on-failure
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF
```

```bash
# Start the service
sudo systemctl daemon-reload
sudo systemctl enable bitcoind
sudo systemctl start bitcoind
```

```bash
# Check the status and logs of the service
systemctl status bitcoind
journalctl -u bitcoind -f
```

### 2.3. Create legacy wallet and generate address:

#### 2.3.1. Create a legacy wallet:

```bash
~/bitcoin-26.0/bin/bitcoin-cli -signet \
    -rpcuser=<your_rpc_username> \
    -rpcpassword=<your_rpc_password> \
    -rpcport=38332 \
    -named createwallet \
    wallet_name=btcstaker \
    passphrase="<passphrase>" \
    load_on_startup=true \
    descriptors=false
```

- Ensure you use the same rpc `rpcuser`, `rpcpassword`, `rpcport` that you used while
  setting up the bitcoind systemd service.
- `-named createwallet` indicates that a new wallet should be created with the
  provided name.
- `wallet_name=btcstaker` specifies the name of the new wallet and `<passphrase>`
  corresponds to the wallet pass phrase. Ensure you use the wallet name and
  passphrase configured here in the [walletconfig](#btc-wallet-configuration)
  section of the `stakerd.conf` file.
- Setting `load_on_startup=true` ensures that the wallet automatically loads during
  system startup.
- `descriptors=false` disables descriptors, which are not currently supported by the
  btc-staker.

#### 2.3.2. Load the wallet:

You can load the wallet with the `loadwallet` command:

```bash
~/bitcoin-26.0/bin/bitcoin-cli -signet \
    -rpcuser=<your_rpc_username> \
    -rpcpassword=<your_rpc_password> \
    -rpcport=38332 \
    loadwallet "btcstaker"
```

where `rpcuser`, `rpcpassword`, and `rpcport` correspond to the RPC configuration you
have set up and `"btcstaker"` should be replaced with the wallet name that you
created.

#### 2.3.3 Generate a new address for the wallet

You can generate a btc address through the `getnewaddress` command:

```bash
~/bitcoin-26.0/bin/bitcoin-cli -signet \
    -rpcuser=<your_rpc_username> \
    -rpcpassword=<your_rpc_password> \
    -rpcport=38332 \
    getnewaddress
```

where `rpcuser`, `rpcpassword`, and `rpcport` correspond to the RPC configuration you
have set up.

### 2.4. Request signet BTC from faucet:

Use our [Discord #faucet-signet-btc channel](https://discord.gg/babylonglobal)
to request signet BTC to the address
generated in the previous step. You can use the following commands if you have
received the funds

You can immediately see the amount using `getunconfirmedbalance`

```bash

~/bitcoin-26.0/bin/bitcoin-cli -signet \
    -rpcuser=<your_rpc_username> \
    -rpcpassword=<your_rpc_password> \
    -rpcport=38332 \
    getunconfirmedbalance
```

You can also see info about the transaction that the faucet gave you
using `gettransaction`

```bash
~/bitcoin-26.0/bin/bitcoin-cli -signet \
    -rpcuser=<your_rpc_username> \
    -rpcpassword=<your_rpc_password> \
    -rpcport=38332 \
    gettransaction $TXID
````

where `$TXID` is the transaction id that you received from the faucet.

Once the tx is confirmed you can check the funds using `getbalance`
command

```bash
~/bitcoin-26.0/bin/bitcoin-cli -signet \
    -rpcuser=<your_rpc_username> \
    -rpcpassword=<your_rpc_password> \
    -rpcport=38332 \
    getbalance
```

**Notes**:

1. Ensure to run the Bitcoin node on the same network as the one the Babylon node
   connects to. For the Babylon testnet, we are using the BTC Signet.
2. Expected sync times for the BTC node are as follows: Signet takes less than 20
   minutes, testnet takes a few hours, and mainnet could take a few days.
3. You can check the sync progress in bitcoind systemd logs
   using `journalctl -u bitcoind -f`. It should show you the progress percentage for
   example it is `progress=0.936446` in this log
   ```bash
   Jan 29 18:55:50 ip-172-31-85-49 bitcoind[71096]:
   2024-01-29T18:55:50Z UpdateTip: new best=00000123354567a29574e6bdd263409b8eab6c05c6ef2abad959b092bf61fe9a
   height=169100 version=0x20000000 log2_work=40.925924 tx=2319364
   date='2023-11-12T19:42:53Z' progress=0.936446
   cache=255.6MiB(1455996txo)
   ```
   Alternatively, you can also check the latest block in a btc explorer like
   https://mempool.space/signet and compare it with the latest block in your node.
4. Ensure that you use a legacy (non-descriptor) wallet, as BTC Staker doesn't
   currently support descriptor wallets. You can check the wallet format using
   ```bash
    ~/bitcoin-26.0/bin/bitcoin-cli -signet \
     -rpcuser=<your_rpc_username> \
     -rpcpassword=<your_rpc_password> \
     -rpcport=38332 \
     getwalletinfo
    ```
   The output should be similar to this and the `format` should be `bdb`:
   ```bash  
   {
     "walletname": "btcstaker",
     "walletversion": 169900,
     "format": "bdb",
     "balance": 0.00000000,
     "unconfirmed_balance": 0.00000000,
     "immature_balance": 0.00000000,
     "txcount": 0,
     "keypoololdest": 1706554908,
     "keypoolsize": 1000,
     "hdseedid": "9660319ab465abc05db95ad17cb59a9ec8f106fd",
     "keypoolsize_hd_internal": 1000,
     "unlocked_until": 0,
     "paytxfee": 0.00000000,
     "private_keys_enabled": true,
     "avoid_reuse": false,
     "scanning": false,
     "descriptors": false,
     "external_signer": false
   }
   ```
5. You can also use `bitcoin.conf` instead of using flags in the `bitcoind` cmd.
   Please check the Bitcoin signet [wiki](https://en.bitcoin.it/wiki/Signet) and this
   manual [here](https://manpages.org/bitcoinconf/5) to learn how to
   set `bitcoin.conf`. Ensure you have configured the `bitcoind.conf` correctly and
   set all the required parameters as shown in the systemd service file above.
