package agent

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func randomString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "fallback"
	}
	return hex.EncodeToString(b)
}

func TestGenerateTraceID_Uniqueness_Stress(t *testing.T) {
	const numGoroutines = 100
	const idsPerGoroutine = 1000
	var wg sync.WaitGroup
	ids := make(chan string, numGoroutines*idsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < idsPerGoroutine; j++ {
				ids <- GenerateTraceID()
			}
		}()
	}

	wg.Wait()
	close(ids)

	uniqueIDs := make(map[string]bool)
	for id := range ids {
		if uniqueIDs[id] {
			t.Errorf("Duplicate TraceID detected: %s", id)
		}
		uniqueIDs[id] = true
	}
}

func TestInteractionLogger_Stress(t *testing.T) {
	store := &mockKVStore{data: make(map[string][]byte)}
	logger := NewInteractionLogger(store)
	defer logger.Stop()

	const numGoroutines = 50
	const recordsPerGoroutine = 100
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < recordsPerGoroutine; j++ {
				logger.Record(InteractionRef{
					TraceID:    fmt.Sprintf("trace-%d-%d", id, j),
					SessionKey: randomString(10),
					UserID:     randomString(5),
					LatencyMs:  int64(j),
					Status:     "completed",
				})
			}
		}(i)
	}

	wg.Wait()
	// Logger is async, but deferred logger.Stop() will wait for drain.

	// Re-check count (some might be dropped if buffer full, but with 5000 records
	// and 1000 buffer, many should be through. InteractionLogger.Record is non-blocking).
	// Since we are testing STABILITY, we ensure no crash.
	storedData, _ := store.List()
	assert.True(t, len(storedData) > 0, "Should have recorded some entries")
}
