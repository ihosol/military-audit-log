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
