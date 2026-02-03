package core

// DocumentMetrics captures fine-grained timing and timestamp data for a single request.
// All timestamps are Unix nanoseconds (UTC implied). Zero means "not measured / not applicable".
//
// This struct is intentionally flat and CSV-friendly for paper experiments.
type DocumentMetrics struct {
	// Whole-request timing (in-process)
	ReqStartUnixNS int64
	ReqEndUnixNS   int64

	// File read
	ReadStartUnixNS int64
	ReadEndUnixNS   int64

	// Hashing
	HashStartUnixNS int64
	HashEndUnixNS   int64

	// Object storage (or filesystem) write
	StorageStartUnixNS int64
	StorageEndUnixNS   int64

	// Merkle batching (only when merkle enabled)
	MerkleEnqueueUnixNS     int64
	MerkleFlushStartUnixNS  int64
	MerkleBuildStartUnixNS  int64
	MerkleBuildEndUnixNS    int64
	MerkleLedgerStartUnixNS int64
	MerkleLedgerEndUnixNS   int64
	MerkleResponseUnixNS    int64
	MerkleLeafIndex         int
	MerkleBatchSize         int

	// Ledger write (per-document, only when merkle disabled and blockchain enabled)
	LedgerStartUnixNS int64
	LedgerEndUnixNS   int64

	// DB save
	DBStartUnixNS int64
	DBEndUnixNS   int64

	// Convenience derived values (seconds). These are duplicated so CSV export is trivial.
	TotalSec        float64
	ReadSec         float64
	HashSec         float64
	StorageSec      float64
	LedgerSec       float64
	DBSec           float64
	MerkleWaitSec   float64
	MerkleBuildSec  float64
	MerkleLedgerSec float64
}
