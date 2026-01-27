#!/bin/bash

# 1. Створення структури папок
echo "Creating project structure..."
mkdir -p cmd
mkdir -p internal/core
mkdir -p internal/ledger
mkdir -p internal/storage
mkdir -p internal/db
mkdir -p deploy

# 2. Ініціалізація Go модуля
echo "Initializing Go module..."
go mod init audit-log

# 3. Створення Docker Compose
cat <<EOF > deploy/docker-compose.yml
version: '3.8'
services:
  minio:
    image: minio/minio
    container_name: audit-minio
    ports:
      - "9000:9000"
      - "9001:9001"
    environment:
      MINIO_ROOT_USER: "admin"
      MINIO_ROOT_PASSWORD: "password123"
    command: server /data --console-address ":9001"
  
  postgres:
    image: postgres:15-alpine
    container_name: audit-postgres
    ports:
      - "5432:5432"
    environment:
      POSTGRES_USER: "user"
      POSTGRES_PASSWORD: "password"
      POSTGRES_DB: "audit_db"
EOF

# 4. Створення Core логіки (Service)
cat <<EOF > internal/core/service.go
package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"time"
)

type Document struct {
	ID            string
	Filename      string
	FileHash      string
	StoragePath   string
	BlockchainTxID string
	CreatedAt     time.Time
}

type ObjectStorage interface {
	Upload(filename string, data io.Reader, size int64) (path string, err error)
}

type Ledger interface {
	Write(hash string, metadata string) (txID string, err error)
}

type Database interface {
	Save(doc *Document) error
}

type AuditService struct {
	storage ObjectStorage
	ledger  Ledger
	db      Database
}

func NewAuditService(s ObjectStorage, l Ledger, d Database) *AuditService {
	return &AuditService{storage: s, ledger: l, db: d}
}

func (s *AuditService) ProcessDocument(filename string, data io.ReadSeeker, size int64) (*Document, error) {
	hashCalculator := sha256.New()
	if _, err := io.Copy(hashCalculator, data); err != nil {
		return nil, fmt.Errorf("hashing error: %v", err)
	}
	fileHash := hex.EncodeToString(hashCalculator.Sum(nil))
	data.Seek(0, 0)

	startBlockchain := time.Now()
	txID, err := s.ledger.Write(fileHash, fmt.Sprintf("File: %s", filename))
	if err != nil {
		return nil, fmt.Errorf("blockchain write error: %v", err)
	}
	fmt.Printf("Blockchain write latency: %v\n", time.Since(startBlockchain))

	path, err := s.storage.Upload(filename, data, size)
	if err != nil {
		return nil, fmt.Errorf("storage upload error: %v", err)
	}

	doc := &Document{
		ID:             fmt.Sprintf("doc-%d", time.Now().UnixNano()),
		Filename:       filename,
		FileHash:       fileHash,
		StoragePath:    path,
		BlockchainTxID: txID,
		CreatedAt:      time.Now(),
	}

	if err := s.db.Save(doc); err != nil {
		return nil, fmt.Errorf("db save error: %v", err)
	}

	return doc, nil
}
EOF

# 5. Створення Mock Ledger (щоб працювало без Fabric спочатку)
cat <<EOF > internal/ledger/mock.go
package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

type MockLedger struct {}

func NewMockLedger() *MockLedger {
	return &MockLedger{}
}

func (m *MockLedger) Write(hash string, metadata string) (string, error) {
	time.Sleep(200 * time.Millisecond) // Simulating network delay
	dummyTx := sha256.Sum256([]byte(hash + time.Now().String()))
	return "0x" + hex.EncodeToString(dummyTx[:]), nil
}
EOF

# 6. Створення Main файлу
cat <<EOF > cmd/main.go
package main

import (
	"audit-log/internal/core"
	"audit-log/internal/ledger"
	"bytes"
	"fmt"
	"log"
	"time"
	"io"
)

// Dummies for MVP start
type DummyStorage struct{}
func (d *DummyStorage) Upload(n string, r io.Reader, s int64) (string, error) { return "/minio/" + n, nil }

type DummyDB struct{}
func (d *DummyDB) Save(doc *core.Document) error {
	fmt.Printf("-> [SQL] Saved: ID=%s, TxID=%s\n", doc.ID, doc.BlockchainTxID)
	return nil
}

func main() {
	fmt.Println("Starting Military Audit Log MVP...")

	// Components
	myLedger := ledger.NewMockLedger()
	myStorage := &DummyStorage{}
	myDB := &DummyDB{}

	service := core.NewAuditService(myStorage, myLedger, myDB)

	// Simulation
	content := []byte("Top Secret Order")
	doc, err := service.ProcessDocument("order.pdf", bytes.NewReader(content), int64(len(content)))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Success! Hash: %s\n", doc.FileHash)
}
EOF

# 7. Створення .gitignore
cat <<EOF > .gitignore
# Binaries
/bin
/dist

# Go
go.sum
vendor/

# Environment
.env

# IDE
.idea/
.vscode/
EOF

echo "Done! Project generated."
