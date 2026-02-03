package core

import (
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

type batchItem struct {
	leaf           []byte
	enqueuedUnixNS int64
	resp           chan MerkleBatchResult
}

// MerkleBatchResult is returned to each Add() caller once the batch is committed to the ledger.
// It also contains timestamps that allow reconstructing queueing / flush / ledger time.
type MerkleBatchResult struct {
	Root      string
	TxID      string
	Index     int
	BatchSize int
	Proof     []MerkleProofStep
	Err       string

	// Timing (Unix ns)
	EnqueueUnixNS     int64
	FlushStartUnixNS  int64
	BuildStartUnixNS  int64
	BuildEndUnixNS    int64
	LedgerStartUnixNS int64
	LedgerEndUnixNS   int64
	ResponseUnixNS    int64
}

type MerkleBatcher struct {
	ledger    Ledger
	batchSize int
	maxWait   time.Duration

	in   chan *batchItem
	stop chan struct{}
	done chan struct{}
}

func NewMerkleBatcher(ledger Ledger, batchSize int, maxWait time.Duration) *MerkleBatcher {
	if batchSize < 1 {
		batchSize = 1
	}
	if maxWait <= 0 {
		maxWait = 25 * time.Millisecond
	}
	b := &MerkleBatcher{
		ledger:    ledger,
		batchSize: batchSize,
		maxWait:   maxWait,
		in:        make(chan *batchItem, batchSize*4),
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}
	go b.loop()
	return b
}

func (b *MerkleBatcher) Close() {
	select {
	case <-b.stop:
		// already closed
	default:
		close(b.stop)
		<-b.done
	}
}

// Add enqueues a leaf hash (raw 32-byte sha256) and blocks until the batch is committed.
func (b *MerkleBatcher) Add(leaf []byte) (MerkleBatchResult, error) {
	if len(leaf) != 32 {
		return MerkleBatchResult{}, errors.New("leaf hash must be 32 bytes (raw sha256)")
	}
	it := &batchItem{
		leaf:           leaf,
		enqueuedUnixNS: time.Now().UnixNano(),
		resp:           make(chan MerkleBatchResult, 1),
	}
	select {
	case b.in <- it:
		// ok
	case <-b.stop:
		return MerkleBatchResult{}, fmt.Errorf("merkle batcher stopped")
	}
	res := <-it.resp
	if res.Err != "" {
		return MerkleBatchResult{}, fmt.Errorf(res.Err)
	}
	return res, nil
}

func (b *MerkleBatcher) loop() {
	defer close(b.done)

	var batch []*batchItem
	var timer *time.Timer
	var timerC <-chan time.Time

	flush := func(items []*batchItem) {
		if len(items) == 0 {
			return
		}

		flushStart := time.Now()
		flushStartNS := flushStart.UnixNano()

		leaves := make([][]byte, 0, len(items))
		for _, it := range items {
			leaves = append(leaves, it.leaf)
		}

		buildStart := time.Now()
		levels, err := buildMerkleLevels(leaves)
		buildEnd := time.Now()

		if err != nil {
			for _, it := range items {
				it.resp <- MerkleBatchResult{
					Err:           err.Error(),
					EnqueueUnixNS: it.enqueuedUnixNS,
				}
			}
			return
		}

		root := levels[len(levels)-1][0]
		rootHex := hex.EncodeToString(root)
		meta := fmt.Sprintf(
			"type=merkle_batch; root=%s; leaves=%d; leaf_algo=sha256(file_bytes); node_algo=sha256(l||r); created_at=%s",
			rootHex, len(leaves), time.Now().UTC().Format(time.RFC3339Nano),
		)

		ledgerStart := time.Now()
		txID, err := b.ledger.Write(rootHex, meta)
		ledgerEnd := time.Now()

		if err != nil {
			for _, it := range items {
				it.resp <- MerkleBatchResult{
					Err:               err.Error(),
					EnqueueUnixNS:     it.enqueuedUnixNS,
					FlushStartUnixNS:  flushStartNS,
					BuildStartUnixNS:  buildStart.UnixNano(),
					BuildEndUnixNS:    buildEnd.UnixNano(),
					LedgerStartUnixNS: ledgerStart.UnixNano(),
					LedgerEndUnixNS:   ledgerEnd.UnixNano(),
					ResponseUnixNS:    time.Now().UnixNano(),
				}
			}
			return
		}

		for i, it := range items {
			proof, _ := merkleProof(levels, i)
			it.resp <- MerkleBatchResult{
				Root:      rootHex,
				TxID:      txID,
				Index:     i,
				BatchSize: len(items),
				Proof:     proof,

				EnqueueUnixNS:     it.enqueuedUnixNS,
				FlushStartUnixNS:  flushStartNS,
				BuildStartUnixNS:  buildStart.UnixNano(),
				BuildEndUnixNS:    buildEnd.UnixNano(),
				LedgerStartUnixNS: ledgerStart.UnixNano(),
				LedgerEndUnixNS:   ledgerEnd.UnixNano(),
				ResponseUnixNS:    time.Now().UnixNano(),
			}
		}
	}

	for {
		select {
		case it := <-b.in:
			batch = append(batch, it)
			if len(batch) == 1 {
				timer = time.NewTimer(b.maxWait)
				timerC = timer.C
			}
			if len(batch) >= b.batchSize {
				if timer != nil {
					_ = timer.Stop()
				}
				flush(batch)
				batch = nil
				timer = nil
				timerC = nil
			}

		case <-timerC:
			flush(batch)
			batch = nil
			timer = nil
			timerC = nil

		case <-b.stop:
			if timer != nil {
				_ = timer.Stop()
			}
			flush(batch)
			return
		}
	}
}
