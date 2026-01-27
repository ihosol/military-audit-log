# Secure Military Audit Log (MVP)

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)
![Docker](https://img.shields.io/badge/Docker-Enabled-2496ED?style=flat&logo=docker)
![Status](https://img.shields.io/badge/Status-Research%20MVP-orange)

A hybrid blockchain architecture implementation for secure, tamper-evident military audit logs. This project demonstrates how to combine **Off-chain Storage** (for large files) with **On-chain Integrity** (for immutable proofs) to ensure data non-repudiation in resource-constrained environments.

## 📖 Overview

In military logistics and command chains, data integrity is paramount. However, storing large documents (orders, maps, supply logs) directly on a blockchain is inefficient and slow.

This solution implements the **"Triangle Architecture"**:
1.  **Storage**: The actual file is stored in a private, S3-compatible object storage (MinIO).
2.  **Ledger**: Only the cryptographic hash (SHA-256) and metadata are written to the Blockchain.
3.  **Index**: A SQL database links the file path to the blockchain transaction ID for fast retrieval.

## 🏗 System Architecture

The MVP currently simulates the entire lifecycle of a secure document:

1.  **Ingestion**: A 10MB dummy file (simulating a high-res map or scan) is generated.
2.  **Hashing**: A unique SHA-256 fingerprint is calculated locally.
3.  **Anchoring**: The hash is sent to the Ledger (currently mocked with network latency simulation).
4.  **Storage**: The physical file is uploaded to the private MinIO cluster.
5.  **Indexing**: Metadata is saved to the local database.

## 🚀 Prerequisites

Before running the project, ensure you have the following installed:

*   **Go**: Version 1.21 or higher.
*   **Docker** & **Docker Compose**.

## 🛠 Quick Start

### 1. Clone the Repository
```bash
git clone https://github.com/your-username/military-audit-log.git
cd military-audit-log
```

### 2. Start Infrastructure
We use Docker to spin up the local Object Storage (MinIO) and Database.

```bash
docker-compose -f deploy/docker-compose.yml up -d
```
*Wait a few seconds for the containers to initialize.*

### 3. Run the Application
The application will generate test data, process it, and output performance metrics.

```bash
# Download dependencies
go mod tidy

# Run the entry point
go run cmd/main.go
```

### 4. Expected Output
You should see logs indicating the processing steps and performance benchmarks:

```text
Starting Military Audit Log MVP...
Created bucket: military-logs
Generating 10MB dummy file...
Processing document...
Blockchain write latency: 200.83ms
-> [SQL] Saved metadata for: doc-176954...

Success! Document saved to MinIO.
Hash: e5b844cc57f57094ea4585e235f36c78...
Total execution time (10MB file): 350.5ms
```

## 🔍 Verification

You can verify that the actual file was securely stored by accessing the MinIO Console.

1.  Open your browser at **[http://localhost:9001](http://localhost:9001)**
2.  Login with credentials:
    *   **Username:** `admin`
    *   **Password:** `password123`
3.  Navigate to **Object Browser** -> **military-logs**.
4.  You will see the uploaded `secret_map_10mb.pdf`.

## 📂 Project Structure

```text
.
├── cmd/
│   └── main.go           # Application entry point & simulation logic
├── deploy/
│   └── docker-compose.yml # Infrastructure definition (MinIO, Postgres)
├── internal/
│   ├── core/             # Core business logic (Service, Domain models)
│   ├── ledger/           # Blockchain interface (Mock / Fabric adapter)
│   ├── storage/          # Object storage implementation (MinIO SDK)
│   └── db/               # Database interactions
├── go.mod                # Go module definitions
└── README.md             # Project documentation
```

## 🔮 Future Roadmap

*   [ ] Integration with **Hyperledger Fabric** Test Network (replacing the Mock Ledger).
*   [ ] Implementation of **CP-ABE** (Attribute-Based Encryption) for role-based access control.
*   [ ] Performance benchmarking suite for scientific paper analysis.

## 🔧 Troubleshooting

*   **Error: `permission denied ... /var/run/docker.sock`**:
    Run the command with `sudo` or add your user to the docker group: `sudo usermod -aG docker $USER`.
*   **Error: `package ... is not in GOROOT`**:
    Your Go version is too old. Please update to Go 1.21+.

---
*Developed for Academic Research on Secure Distributed Systems.*