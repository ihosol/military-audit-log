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
