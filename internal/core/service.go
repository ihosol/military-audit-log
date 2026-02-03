package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
	"io"
	"bytes"

)

type Document struct {
	ID          string
	StoragePath string
	HashHex     string
	TxID        string

	// Merkle batching
	MerkleRoot      string
	MerkleLeafIndex int
	MerkleBatchSize int

	CreatedAt time.Time
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
	store  ObjectStorage
	db     Database
	ledger Ledger

	useBC  bool
	merkle *MerkleBatcher
}

func NewAuditService(store ObjectStorage, db Database, ledger Ledger, useBC bool) *AuditService {
	return &AuditService{store: store, db: db, ledger: ledger, useBC: useBC}
}

func (s *AuditService) EnableMerkleBatching(batcher *MerkleBatcher) {
	s.merkle = batcher
}

// ProcessDocument stores the raw content in object storage, stores metadata in DB, and (optionally)
// commits either the document hash (direct) or a merkle root (batched) to the ledger.
//
// It returns a Document, fine-grained DocumentMetrics, and an error (if any).
func (s *AuditService) ProcessDocument(content []byte) (*Document, *DocumentMetrics, error) {
	m := &DocumentMetrics{}
	reqStart := time.Now()
	m.ReqStartUnixNS = reqStart.UnixNano()

	doc := &Document{
		ID:        fmt.Sprintf("doc-%d", time.Now().UnixNano()),
		CreatedAt: time.Now(),
	}

	// --- Hash ---
	h0 := time.Now()
	m.HashStartUnixNS = h0.UnixNano()
	sum := sha256.Sum256(content)
	rawHash := sum[:]
	doc.HashHex = hex.EncodeToString(rawHash)
	h1 := time.Now()
	m.HashEndUnixNS = h1.UnixNano()
	m.HashSec = h1.Sub(h0).Seconds()

	// --- Object storage ---
	s0 := time.Now()
	m.StorageStartUnixNS = s0.UnixNano()
	path := fmt.Sprintf("%s.bin", doc.ID)
	path, err := s.store.Upload(path,  bytes.NewReader(content), int64(len(content)))
	if err != nil {
		m.ReqEndUnixNS = time.Now().UnixNano()
		m.TotalSec = time.Since(reqStart).Seconds()
		return nil, m, err
	}
	m.StorageEndUnixNS = time.Now().UnixNano()
	m.StorageSec = time.Duration(m.StorageEndUnixNS - m.StorageStartUnixNS).Seconds()
	doc.StoragePath = path

	// --- DB ---
	d0 := time.Now()
	m.DBStartUnixNS = d0.UnixNano()
	if err := s.db.Save(doc); err != nil {
		m.DBEndUnixNS = time.Now().UnixNano()
		m.DBSec = time.Duration(m.DBEndUnixNS - m.DBStartUnixNS).Seconds()
		m.ReqEndUnixNS = time.Now().UnixNano()
		m.TotalSec = time.Since(reqStart).Seconds()
		return nil, m, err
	}
	m.DBEndUnixNS = time.Now().UnixNano()
	m.DBSec = time.Duration(m.DBEndUnixNS - m.DBStartUnixNS).Seconds()

	if s.useBC {
		if s.merkle != nil {
			// --- Merkle batch enqueue + wait ---
			m.MerkleEnqueueUnixNS = time.Now().UnixNano()
			res, err := s.merkle.Add(rawHash)
			if err != nil {
				m.ReqEndUnixNS = time.Now().UnixNano()
				m.TotalSec = time.Since(reqStart).Seconds()
				return nil, m, err
			}
			doc.MerkleRoot = res.Root
			doc.MerkleLeafIndex = res.Index
			doc.MerkleBatchSize = res.BatchSize
			doc.TxID = res.TxID

			// propagate timings from batcher
			m.MerkleFlushStartUnixNS = res.FlushStartUnixNS
			m.MerkleBuildStartUnixNS = res.BuildStartUnixNS
			m.MerkleBuildEndUnixNS = res.BuildEndUnixNS
			m.MerkleLedgerStartUnixNS = res.LedgerStartUnixNS
			m.MerkleLedgerEndUnixNS = res.LedgerEndUnixNS
			m.MerkleResponseUnixNS = res.ResponseUnixNS

			m.MerkleLeafIndex = res.Index
			m.MerkleBatchSize = res.BatchSize

			if m.MerkleResponseUnixNS > 0 && res.EnqueueUnixNS > 0 {
				m.MerkleWaitSec = time.Duration(m.MerkleResponseUnixNS - res.EnqueueUnixNS).Seconds()
			}
			if m.MerkleBuildEndUnixNS > 0 && m.MerkleBuildStartUnixNS > 0 {
				m.MerkleBuildSec = time.Duration(m.MerkleBuildEndUnixNS - m.MerkleBuildStartUnixNS).Seconds()
			}
			if m.MerkleLedgerEndUnixNS > 0 && m.MerkleLedgerStartUnixNS > 0 {
				m.MerkleLedgerSec = time.Duration(m.MerkleLedgerEndUnixNS - m.MerkleLedgerStartUnixNS).Seconds()
			}
		} else {
			// --- Direct ledger write ---
			l0 := time.Now()
			m.LedgerStartUnixNS = l0.UnixNano()
			txID, err := s.ledger.Write(doc.HashHex, "")
			m.LedgerEndUnixNS = time.Now().UnixNano()
			m.LedgerSec = time.Duration(m.LedgerEndUnixNS - m.LedgerStartUnixNS).Seconds()
			if err != nil {
				m.ReqEndUnixNS = time.Now().UnixNano()
				m.TotalSec = time.Since(reqStart).Seconds()
				return nil, m, err
			}
			doc.TxID = txID
		}
	}

	m.ReqEndUnixNS = time.Now().UnixNano()
	m.TotalSec = time.Since(reqStart).Seconds()
	return doc, m, nil
}
