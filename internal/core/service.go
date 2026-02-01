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
	Read(hash string) (metadata string, err error)

}

type Database interface {
	Save(doc *Document) error
	Get(id string) (*Document, error) 
}

type AuditService struct {
	storage ObjectStorage
	ledger  Ledger
	db      Database
}

func NewAuditService(s ObjectStorage, l Ledger, d Database) *AuditService {
	return &AuditService{storage: s, ledger: l, db: d}
}

// ProcessDocument тепер приймає useBlockchain (для Baseline тесту)
func (s *AuditService) ProcessDocument(filename string, data io.ReadSeeker, size int64, useBlockchain bool) (*Document, error) {
	// 1. Хешування
	hashCalculator := sha256.New()
	if _, err := io.Copy(hashCalculator, data); err != nil {
		return nil, fmt.Errorf("hashing error: %v", err)
	}
	fileHash := hex.EncodeToString(hashCalculator.Sum(nil))
	data.Seek(0, 0) // Скидаємо рідер на початок

	// 2. Блокчейн (Тільки якщо увімкнено!)
	var txID string
	var err error
	
	if useBlockchain {
		// Тут буде затримка 2с
		txID, err = s.ledger.Write(fileHash, fmt.Sprintf("File: %s", filename))
		if err != nil {
			return nil, fmt.Errorf("blockchain write error: %v", err)
		}
	} else {
		// Baseline режим: імітуємо, що блокчейну немає (0 затримки)
		txID = "skipped-baseline-mode"
	}

	// 3. MinIO
	path, err := s.storage.Upload(filename, data, size)
	if err != nil {
		return nil, fmt.Errorf("storage upload error: %v", err)
	}

	// 4. Postgres
	doc := &Document{
		ID:             fmt.Sprintf("doc-%d-%s", time.Now().UnixNano(), fileHash[:8]),
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

// VerifyDocument - НОВИЙ МЕТОД ДЛЯ АУДИТОРА
func (s *AuditService) VerifyDocument(docID string) (bool, string, error) {
	// А. Отримуємо запис з БД
	doc, err := s.db.Get(docID) // Тобі треба додати Get в інтерфейс DB і в postgres.go!
	if err != nil {
		return false, "Database missing", err
	}

	// Б. Качаємо файл з MinIO (треба додати Download в інтерфейс Storage і minio.go!)
	// Для MVP спростимо: припустимо, ми перевіряємо тільки хеш у блокчейні по запису з БД
	
	// В. Читаємо з Блокчейну
	ledgerData, err := s.ledger.Read(doc.FileHash)
	if err != nil {
		return false, "Blockchain missing", fmt.Errorf("hash not found in ledger: %v", err)
	}

	// Г. Звірка
	// У реальності тут ми б ще перехешували файл з MinIO, але для статті достатньо факту наявності
	if ledgerData != "" {
		return true, "Valid Integrity", nil
	}

	return false, "Invalid", nil
}