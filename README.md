# Secure Military Audit Log (Full MVP)

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)
![Hyperledger Fabric](https://img.shields.io/badge/Hyperledger%20Fabric-2.5-black?style=flat&logo=hyperledger)
![Status](https://img.shields.io/badge/Status-Research%20Prototype-success)

A hybrid blockchain architecture implementation for secure, tamper-evident military audit logs. This project integrates **Hyperledger Fabric** (for immutable proofs) with **MinIO** (for secure off-chain storage) to solve the "Triangle Architecture" challenge in distributed logistics.

## 📖 Overview

This system provides a **non-repudiation mechanism** for military orders and supply logs.
1.  **Off-chain**: Large files (PDFs, maps) are stored in MinIO (S3-compatible).
2.  **On-chain**: The file's SHA-256 hash and metadata are anchored in a private Hyperledger Fabric network.
3.  **Result**: 100% data integrity with verifiable history, without clogging the blockchain with large data payloads.

## 🏗 System Architecture

The project requires two parallel components running locally:

1.  **The Application**: Go backend + MinIO + Postgres (Docker Compose).
2.  **The Network**: Hyperledger Fabric Test Network (Peer Nodes + Orderer).

```text
[ Client App ]  --->  [ MinIO Storage (Docker) ]
      |
      +------------>  [ Fabric Gateway ]
                            |
                     [ Peer0.Org1 ] <---> [ Orderer ]
```

## 🚀 Prerequisites

*   **OS**: Linux (Ubuntu recommended) or WSL2 on Windows.
*   **Go**: Version 1.21+.
*   **Docker** & **Docker Compose**.
*   **Curl** & **Git**.

## 🛠 Installation & Setup

### 1. Directory Structure Setup
To ensure the hardcoded paths in the Go code work, we recommend this folder structure:

```text
~/code/
  ├── military-audit-log/       # This repository
  └── fabric-network/           # Hyperledger Fabric infrastructure
```

### 2. Start Hyperledger Fabric Network
If you haven't installed Fabric yet, run these commands:

```bash
# Create infrastructure folder
mkdir -p ~/code/fabric-network && cd ~/code/fabric-network

# Download Fabric Docker images and binaries (Version 2.5.x)
curl -sSLO https://raw.githubusercontent.com/hyperledger/fabric/main/scripts/install-fabric.sh && chmod +x install-fabric.sh
./install-fabric.sh --fabric-version 2.5.9 binary samples docker

# Start the Network and Deploy Chaincode
cd fabric-samples/test-network
./network.sh down
./network.sh up createChannel -c mychannel -ca
./network.sh deployCC -ccn basic -ccp ../asset-transfer-basic/chaincode-go -ccl go
```

### 3. Start Application Infrastructure
Go back to the project folder and start MinIO and Postgres.

```bash
cd ~/code/military-audit-log
docker-compose -f deploy/docker-compose.yml up -d
```

## ⚡ Running the Benchmark

The application is configured to run a performance benchmark (writing files to Storage + Blockchain sequentially).

```bash
# Install Go dependencies
go mod tidy

# Run the full MVP
go run cmd/main.go
```

### Expected Output
You will see the latency of real consensus (typically ~2 seconds per block).

```text
🚀 Starting Benchmark for Research Paper...
Running 10 iterations with 1048576 byte files...
[1/10] Processing... Done in 2.1500s
[2/10] Processing... Done in 2.0800s
...
✅ Benchmark finished! Data saved to 'benchmark_results.csv'
```

## 🔍 Verification

### 1. Verify Off-chain Storage (MinIO)
*   **URL**: [http://localhost:9001](http://localhost:9001)
*   **User/Pass**: `admin` / `password123`
*   **Bucket**: `military-logs` (You will see the actual binary files here).

### 2. Verify On-chain Data (Fabric CLI)
You can query the ledger directly from the Docker container to prove the data exists on the blockchain.

```bash
docker exec cli peer chaincode query -C mychannel -n basic -c '{"Args":["QueryAssets", "{\"selector\":{\"docType\":\"asset\"}}"]}'
```
*You will see a JSON output containing your file hashes and timestamps.*

## 🔧 Troubleshooting

**Error: `crypto path does not exist`**
*   The Go code cannot find the Fabric certificates.
*   **Fix**: Open `internal/ledger/fabric.go` and adjust the `cryptoPath` variable to point to your `fabric-samples` directory.

**Error: `rpc error: code = Unavailable`**
*   The Fabric Gateway cannot reach the Peer node.
*   **Fix**: Ensure the network is running (`docker ps` should show `peer0.org1.example.com`).

**Error: `permission denied`**
*   **Fix**: Run commands with `sudo` or add your user to the `docker` group.

---
*Developed for Academic Research on Secure Distributed Systems.*