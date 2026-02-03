package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

// MerkleProofStep represents one step in an inclusion proof.
// Side indicates where the sibling hash sits relative to the running hash:
// - "L" means sibling is on the left: H = sha256(sibling || current)
// - "R" means sibling is on the right: H = sha256(current || sibling)
type MerkleProofStep struct {
	Hash string `json:"hash"`
	Side string `json:"side"`
}

// buildMerkleLevels builds all Merkle levels (level 0 = leaves). If the level has an odd
// number of nodes, the last node is duplicated (Bitcoin-style) to form a pair.
func buildMerkleLevels(leaves [][]byte) ([][][]byte, error) {
	if len(leaves) == 0 {
		return nil, errors.New("no leaves")
	}

	lvl0 := make([][]byte, len(leaves))
	for i := range leaves {
		if len(leaves[i]) == 0 {
			return nil, errors.New("empty leaf")
		}
		// Copy to avoid caller mutation.
		b := make([]byte, len(leaves[i]))
		copy(b, leaves[i])
		lvl0[i] = b
	}

	levels := [][][]byte{lvl0}
	for {
		curr := levels[len(levels)-1]
		if len(curr) == 1 {
			break
		}
		next := make([][]byte, 0, (len(curr)+1)/2)
		for i := 0; i < len(curr); i += 2 {
			left := curr[i]
			right := left
			if i+1 < len(curr) {
				right = curr[i+1]
			}
			buf := make([]byte, 0, len(left)+len(right))
			buf = append(buf, left...)
			buf = append(buf, right...)
			sum := sha256.Sum256(buf)
			parent := make([]byte, len(sum))
			copy(parent, sum[:])
			next = append(next, parent)
		}
		levels = append(levels, next)
	}
	return levels, nil
}

func merkleRootFromLevels(levels [][][]byte) []byte {
	return levels[len(levels)-1][0]
}

// merkleProof returns an inclusion proof for a leaf at `index`.
func merkleProof(levels [][][]byte, index int) ([]MerkleProofStep, error) {
	if len(levels) == 0 {
		return nil, errors.New("no levels")
	}
	if index < 0 || index >= len(levels[0]) {
		return nil, errors.New("leaf index out of range")
	}

	proof := make([]MerkleProofStep, 0, len(levels)-1)
	idx := index
	for lvl := 0; lvl < len(levels)-1; lvl++ {
		nodes := levels[lvl]
		isRight := (idx % 2) == 1
		sibIdx := idx - 1
		side := "L"
		if !isRight {
			sibIdx = idx + 1
			side = "R"
		}
		if sibIdx >= len(nodes) {
			// Odd count: duplicate last.
			sibIdx = idx
		}
		proof = append(proof, MerkleProofStep{
			Hash: hex.EncodeToString(nodes[sibIdx]),
			Side: side,
		})
		idx = idx / 2
	}
	return proof, nil
}

// computeRootFromProof computes the Merkle root implied by leaf + proof.
func computeRootFromProof(leaf []byte, proof []MerkleProofStep) ([]byte, error) {
	if len(leaf) == 0 {
		return nil, errors.New("empty leaf")
	}
	curr := make([]byte, len(leaf))
	copy(curr, leaf)

	for _, step := range proof {
		sib, err := hex.DecodeString(step.Hash)
		if err != nil {
			return nil, errors.New("invalid proof hash encoding")
		}
		var buf []byte
		switch step.Side {
		case "L":
			buf = make([]byte, 0, len(sib)+len(curr))
			buf = append(buf, sib...)
			buf = append(buf, curr...)
		case "R":
			buf = make([]byte, 0, len(curr)+len(sib))
			buf = append(buf, curr...)
			buf = append(buf, sib...)
		default:
			return nil, errors.New("invalid proof side")
		}
		sum := sha256.Sum256(buf)
		next := make([]byte, len(sum))
		copy(next, sum[:])
		curr = next
	}
	return curr, nil
}

// VerifyMerkleProof verifies that (leafHashHex, proofJSON) produces expectedRootHex.
// proofJSON is a JSON array of MerkleProofStep.
func VerifyMerkleProof(leafHashHex string, proofJSON string, expectedRootHex string) (bool, error) {
	leaf, err := hex.DecodeString(leafHashHex)
	if err != nil {
		return false, errors.New("invalid leaf hash encoding")
	}
	expectedRoot, err := hex.DecodeString(expectedRootHex)
	if err != nil {
		return false, errors.New("invalid root hash encoding")
	}

	var proof []MerkleProofStep
	if err := json.Unmarshal([]byte(proofJSON), &proof); err != nil {
		return false, errors.New("invalid proof json")
	}

	computed, err := computeRootFromProof(leaf, proof)
	if err != nil {
		return false, err
	}

	if len(computed) != len(expectedRoot) {
		return false, nil
	}
	for i := range computed {
		if computed[i] != expectedRoot[i] {
			return false, nil
		}
	}
	return true, nil
}
