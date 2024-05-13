# Setup Bitcoin Node

The staking indexer requires a synced `bitcoind` node as the backend.

Below, we provide instructions on setting up a signet `bitcoind` node in `Ubuntu`.

### 1. Download and install bitcoind:

```bash
# Download Bitcoin Core binary
wget https://bitcoincore.org/bin/bitcoin-core-26.0/bitcoin-26.0-x86_64-linux-gnu.tar.gz

# Extract the downloaded archive
tar -xvf bitcoin-26.0-x86_64-linux-gnu.tar.gz

# Provide execution permissions to binaries
chmod +x bitcoin-26.0/bin/bitcoind
chmod +x bitcoin-26.0/bin/bitcoin-cli
```

### 2. Create and start a Systemd Service:

Please update the following configurations in the provided file:

1. Replace `<your_rpc_username>` and `<your_rpc_password>` with your own values.
   These credentials should be utilized in the `sid.conf` configuration file
   generated via `sid init`.
2. Ensure that the `<user>` is set to the machine user. In the guide below, it's set
   to `ubuntu`.
3. If you want to enable remote connections to the node, you can add
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

**Notes**:

1. Expected sync times for the BTC node are as follows: Signet takes less than 20
   minutes, testnet takes a few hours, and mainnet could take a few days.
2. You can check the sync progress in bitcoind systemd logs
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
3. You can also use `bitcoin.conf` instead of using flags in the `bitcoind` cmd.
   Please check the Bitcoin signet [wiki](https://en.bitcoin.it/wiki/Signet) and this
   manual [here](https://manpages.org/bitcoinconf/5) to learn how to
   set `bitcoin.conf`. Ensure you have configured the `bitcoind.conf` correctly and
   set all the required parameters as shown in the systemd service file above.
