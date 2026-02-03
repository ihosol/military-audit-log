package db

import (
	"errors"
	"sync"

	"audit-log/internal/core"
)

// MemoryDB is a simple in-memory implementation of core.Database (useful for unit tests).
type MemoryDB struct {
	mu   sync.RWMutex
	docs map[string]core.Document
}

func NewMemoryDB() *MemoryDB {
	return &MemoryDB{docs: make(map[string]core.Document)}
}

func (m *MemoryDB) Save(doc core.Document) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.docs[doc.ID] = doc
	return nil
}

func (m *MemoryDB) Get(docID string) (core.Document, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	doc, ok := m.docs[docID]
	if !ok {
		return core.Document{}, errors.New("document not found")
	}
	return doc, nil
}
