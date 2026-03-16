package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Key generates a deterministic idempotency key from prefix and payload.
// Useful for batch operations where each logical request should be retry-safe.
func Key(prefix string, payload any) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%s-%s-%d", prefix, hex.EncodeToString(h[:8]), time.Now().Unix()), nil
}

// KeyForBatch generates a key for a batch segment (e.g. "catalog-batch-0-abc123-1234567890").
func KeyForBatch(operation string, batchIndex int, segmentID string) string {
	return fmt.Sprintf("%s-batch-%d-%s-%d", operation, batchIndex, segmentID, time.Now().Unix())
}
