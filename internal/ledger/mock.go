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
