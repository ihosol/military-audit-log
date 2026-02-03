package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"audit-log/internal/core"
	"audit-log/internal/db"
	"audit-log/internal/ledger"
	"audit-log/internal/storage"
)

type ResultRow struct {
	RunID         string
	Mode          string
	Workers       int
	JobID         int
	WorkerID      int
	FileSizeBytes int
	Status        string
	Error         string

	DocID         string
	DocHashHex    string
	StoragePath   string
	TxID          string
	MerkleRoot    string
	MerkleLeafIdx int
	MerkleBatchSz int

	// coarse + fine metrics
	ReqStartUnixNS int64
	ReqEndUnixNS   int64
	TotalSec       float64

	HashStartUnixNS int64
	HashEndUnixNS   int64
	HashSec         float64

	StorageStartUnixNS int64
	StorageEndUnixNS   int64
	StorageSec         float64

	DBStartUnixNS int64
	DBEndUnixNS   int64
	DBSec         float64

	LedgerStartUnixNS int64
	LedgerEndUnixNS   int64
	LedgerSec         float64

	MerkleEnqueueUnixNS     int64
	MerkleFlushStartUnixNS  int64
	MerkleBuildStartUnixNS  int64
	MerkleBuildEndUnixNS    int64
	MerkleLedgerStartUnixNS int64
	MerkleLedgerEndUnixNS   int64
	MerkleResponseUnixNS    int64
	MerkleWaitSec           float64
	MerkleBuildSec          float64
	MerkleLedgerSec         float64
}

func parseSizesCSV(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return []int{1024 * 1024}, nil
	}
	parts := strings.Split(s, ",")
	sizes := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid size '%s' (expected integer bytes)", p)
		}
		if n <= 0 {
			return nil, fmt.Errorf("size must be > 0: %d", n)
		}
		sizes = append(sizes, n)
	}
	if len(sizes) == 0 {
		sizes = []int{1024 * 1024}
	}
	return sizes, nil
}

func main() {
	mode := flag.String("mode", "bench", "baseline|bench") // baseline disables blockchain
	workers := flag.Int("workers", 1, "number of worker goroutines")
	count := flag.Int("count", 10, "number of jobs (documents) to process")
	sizesCSV := flag.String("sizes", "1048576", "comma-separated payload sizes in bytes (e.g., 4096,65536,1048576,5242880)")
	seed := flag.Int64("seed", time.Now().UnixNano(), "random seed for payload generation")
	out := flag.String("out", "", "output CSV filename (default: auto)")

	useMerkle := flag.Bool("merkle", false, "enable merkle batching (only in bench mode)")
	merkleBatch := flag.Int("merkle-batch", 128, "merkle batch size")
	merkleWaitMs := flag.Int("merkle-wait-ms", 50, "max wait in ms before flushing a merkle batch")
	warmup := flag.Int("warmup", 1, "number of initial jobs to mark as warmup (still recorded in CSV)")

	flag.Parse()

	sizes, err := parseSizesCSV(*sizesCSV)
	if err != nil {
		fmt.Println("❌", err)
		os.Exit(1)
	}

	useBC := *mode != "baseline"

	fmt.Printf("🔬 Starting Experiment: Mode=%s | Workers=%d | Jobs=%d | Sizes=%v\n", *mode, *workers, *count, sizes)
	if !useBC {
		fmt.Println("⚠️  BASELINE MODE: Blockchain Disabled")
	}
	if useBC && *useMerkle {
		fmt.Printf("🌳 Merkle batching: batch=%d wait=%dms\n", *merkleBatch, *merkleWaitMs)
	}

	rand.Seed(*seed)

	// Dependencies
	store := storage.NewMinioStorage("localhost:9000", "admin", "password123", "military-logs")
	database, err := db.NewPostgresDB("localhost", "user", "password", "audit_db", "5432")

	var led core.Ledger
	if !useBC {
		led = &ledger.MockLedger{}
	} else {
		led, err = ledger.NewFabricLedger()
	}
	if err != nil {
		fmt.Println("❌ Failed to initialize dependencies:", err)
		os.Exit(1)
	}
	svc := core.NewAuditService(store, database, led, useBC)
	if useBC && *useMerkle {
		batcher := core.NewMerkleBatcher(led, *merkleBatch, time.Duration(*merkleWaitMs)*time.Millisecond)
		defer batcher.Close()
		svc.EnableMerkleBatching(batcher)
	}

	// Output file
	runID := fmt.Sprintf("%d", time.Now().UnixNano())
	outName := *out
	if outName == "" {
		if useBC && *useMerkle {
			outName = fmt.Sprintf("results_%s_merkle_b%d_w%dms_w%d_c%d.csv", *mode, *merkleBatch, *merkleWaitMs, *workers, *count)
		} else {
			outName = fmt.Sprintf("results_%s_w%d_c%d.csv", *mode, *workers, *count)
		}
	}
	f, err := os.Create(outName)
	if err != nil {
		fmt.Println("❌ Failed to create output:", err)
		os.Exit(1)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{
		"run_id", "mode", "workers", "job_id", "worker_id", "file_size_bytes", "is_warmup", "status", "error",
		"doc_id", "doc_hash_hex", "storage_path", "tx_id", "merkle_root", "merkle_leaf_index", "merkle_batch_size",

		"req_start_unix_ns", "req_end_unix_ns", "total_sec",
		"hash_start_unix_ns", "hash_end_unix_ns", "hash_sec",
		"storage_start_unix_ns", "storage_end_unix_ns", "storage_sec",
		"db_start_unix_ns", "db_end_unix_ns", "db_sec",
		"ledger_start_unix_ns", "ledger_end_unix_ns", "ledger_sec",

		"merkle_enqueue_unix_ns", "merkle_flush_start_unix_ns", "merkle_build_start_unix_ns", "merkle_build_end_unix_ns",
		"merkle_ledger_start_unix_ns", "merkle_ledger_end_unix_ns", "merkle_response_unix_ns",
		"merkle_wait_sec", "merkle_build_sec", "merkle_ledger_sec",
	}
	_ = w.Write(header)

	jobs := make(chan int)
	results := make(chan ResultRow, *count)
	var wg sync.WaitGroup

	// Workers
	for wid := 1; wid <= *workers; wid++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for jobID := range jobs {
				size := sizes[(jobID-1)%len(sizes)]
				payload := make([]byte, size)
				_, _ = rand.Read(payload)

				doc, m, err := svc.ProcessDocument(payload)
				row := ResultRow{
					RunID:         runID,
					Mode:          *mode,
					Workers:       *workers,
					JobID:         jobID,
					WorkerID:      workerID,
					FileSizeBytes: size,
				}

				if err != nil {
					row.Status = "error"
					row.Error = err.Error()
				} else {
					row.Status = "ok"
					row.DocID = doc.ID
					row.DocHashHex = doc.HashHex
					row.StoragePath = doc.StoragePath
					row.TxID = doc.TxID
					row.MerkleRoot = doc.MerkleRoot
					row.MerkleLeafIdx = doc.MerkleLeafIndex
					row.MerkleBatchSz = doc.MerkleBatchSize
				}

				if m != nil {
					row.ReqStartUnixNS = m.ReqStartUnixNS
					row.ReqEndUnixNS = m.ReqEndUnixNS
					row.TotalSec = m.TotalSec

					row.HashStartUnixNS = m.HashStartUnixNS
					row.HashEndUnixNS = m.HashEndUnixNS
					row.HashSec = m.HashSec

					row.StorageStartUnixNS = m.StorageStartUnixNS
					row.StorageEndUnixNS = m.StorageEndUnixNS
					row.StorageSec = m.StorageSec

					row.DBStartUnixNS = m.DBStartUnixNS
					row.DBEndUnixNS = m.DBEndUnixNS
					row.DBSec = m.DBSec

					row.LedgerStartUnixNS = m.LedgerStartUnixNS
					row.LedgerEndUnixNS = m.LedgerEndUnixNS
					row.LedgerSec = m.LedgerSec

					row.MerkleEnqueueUnixNS = m.MerkleEnqueueUnixNS
					row.MerkleFlushStartUnixNS = m.MerkleFlushStartUnixNS
					row.MerkleBuildStartUnixNS = m.MerkleBuildStartUnixNS
					row.MerkleBuildEndUnixNS = m.MerkleBuildEndUnixNS
					row.MerkleLedgerStartUnixNS = m.MerkleLedgerStartUnixNS
					row.MerkleLedgerEndUnixNS = m.MerkleLedgerEndUnixNS
					row.MerkleResponseUnixNS = m.MerkleResponseUnixNS
					row.MerkleWaitSec = m.MerkleWaitSec
					row.MerkleBuildSec = m.MerkleBuildSec
					row.MerkleLedgerSec = m.MerkleLedgerSec
				}

				// Console summary
				fmt.Printf("Worker %d: Job %d done in %.2fs\n", workerID, jobID, row.TotalSec)
				results <- row
			}
		}(wid)
	}

	// Feed jobs
	expStart := time.Now()
	for j := 1; j <= *count; j++ {
		jobs <- j
	}
	close(jobs)
	wg.Wait()
	close(results)
	expEnd := time.Now()

	// Write rows
	for r := range results {
		isWarmup := "0"
		if r.JobID <= *warmup {
			isWarmup = "1"
		}
		rec := []string{
			r.RunID,
			r.Mode,
			strconv.Itoa(r.Workers),
			strconv.Itoa(r.JobID),
			strconv.Itoa(r.WorkerID),
			strconv.Itoa(r.FileSizeBytes),
			isWarmup,
			r.Status,
			r.Error,

			r.DocID,
			r.DocHashHex,
			r.StoragePath,
			r.TxID,
			r.MerkleRoot,
			strconv.Itoa(r.MerkleLeafIdx),
			strconv.Itoa(r.MerkleBatchSz),

			strconv.FormatInt(r.ReqStartUnixNS, 10),
			strconv.FormatInt(r.ReqEndUnixNS, 10),
			fmt.Sprintf("%.6f", r.TotalSec),

			strconv.FormatInt(r.HashStartUnixNS, 10),
			strconv.FormatInt(r.HashEndUnixNS, 10),
			fmt.Sprintf("%.6f", r.HashSec),

			strconv.FormatInt(r.StorageStartUnixNS, 10),
			strconv.FormatInt(r.StorageEndUnixNS, 10),
			fmt.Sprintf("%.6f", r.StorageSec),

			strconv.FormatInt(r.DBStartUnixNS, 10),
			strconv.FormatInt(r.DBEndUnixNS, 10),
			fmt.Sprintf("%.6f", r.DBSec),

			strconv.FormatInt(r.LedgerStartUnixNS, 10),
			strconv.FormatInt(r.LedgerEndUnixNS, 10),
			fmt.Sprintf("%.6f", r.LedgerSec),

			strconv.FormatInt(r.MerkleEnqueueUnixNS, 10),
			strconv.FormatInt(r.MerkleFlushStartUnixNS, 10),
			strconv.FormatInt(r.MerkleBuildStartUnixNS, 10),
			strconv.FormatInt(r.MerkleBuildEndUnixNS, 10),
			strconv.FormatInt(r.MerkleLedgerStartUnixNS, 10),
			strconv.FormatInt(r.MerkleLedgerEndUnixNS, 10),
			strconv.FormatInt(r.MerkleResponseUnixNS, 10),
			fmt.Sprintf("%.6f", r.MerkleWaitSec),
			fmt.Sprintf("%.6f", r.MerkleBuildSec),
			fmt.Sprintf("%.6f", r.MerkleLedgerSec),
		}
		_ = w.Write(rec)
	}

	w.Flush()

	measured := expEnd.Sub(expStart).Seconds()
	tps := float64(*count) / measured
	fmt.Println()
	fmt.Println("✅ Experiment Finished!")
	fmt.Printf("Total Time: %.2fs\n", measured)
	fmt.Printf("Throughput (TPS): %.2f req/sec\n", tps)
	fmt.Printf("Data saved to: %s\n", outName)
}
