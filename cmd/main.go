package main

import (
	"audit-log/internal/core"
	"audit-log/internal/ledger"
	"audit-log/internal/storage" // Додали новий імпорт
	"bytes"
	"fmt"
	"log"
	"time"
)

// DummyDB поки залишаємо (наступним кроком замінимо на Postgres)
type DummyDB struct{}
func (d *DummyDB) Save(doc *core.Document) error {
	fmt.Printf("-> [SQL] Saved metadata for: %s\n", doc.ID)
	return nil
}

func main() {
	fmt.Println("Starting Military Audit Log MVP...")

	// 1. Підключаємо REAL MinIO (переконайся, що docker-compose запущений!)
	// endpoint "localhost:9000", user "admin", pass "password123" (з docker-compose.yml)
	realStorage := storage.NewMinioStorage("localhost:9000", "admin", "password123", "military-logs")

	myLedger := ledger.NewMockLedger()
	myDB := &DummyDB{}

	service := core.NewAuditService(realStorage, myLedger, myDB)

	// 2. Емуляція файлу (10 МБ нулів) - щоб перевірити швидкість
	fmt.Println("Generating 10MB dummy file...")
	fileSize := 10 * 1024 * 1024 // 10 MB
	largeContent := make([]byte, fileSize) 
	reader := bytes.NewReader(largeContent)

	// 3. Запуск тесту
	fmt.Println("Processing document...")
	start := time.Now()

	doc, err := service.ProcessDocument("secret_map_10mb.pdf", reader, int64(fileSize))
	if err != nil {
		log.Fatal(err)
	}

	duration := time.Since(start)

	fmt.Printf("\nSuccess! Document saved to MinIO.\n")
	fmt.Printf("Hash: %s\n", doc.FileHash)
	fmt.Printf("Total execution time (10MB file): %v\n", duration)
}