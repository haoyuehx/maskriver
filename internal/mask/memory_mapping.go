package mask

import (
	"context"
	"sync"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// MemoryMapping is an in-process, concurrency-safe implementation of
// both contracts.MappingReader and contracts.MappingWriter. Its primary
// purpose is unit testing the plan runner without a durable mapping
// store; durable stores (SQLite-backed mappings) belong in internal/db
// and are explicitly out of scope for M1-B mask module.
//
// Security invariants:
//   - MappingKey.Fingerprint is the only index value bound to the
//     original input; memory storage never retains raw original
//     Values passed to GetOrCreate.
//   - The value stored is the masked candidate supplied by the
//     caller, which itself must not leak the original payload.
type MemoryMapping struct {
	mu   sync.RWMutex
	data map[contracts.MappingKey]contracts.Value
}

// NewMemoryMapping constructs an empty in-memory mapping store.
func NewMemoryMapping() *MemoryMapping {
	return &MemoryMapping{data: make(map[contracts.MappingKey]contracts.Value)}
}

// Lookup returns the stored masked Value for key, if any. A nil receiver
// or nil backing map always reports not-found without error so that
// callers can treat a zero-initialized store as empty.
func (m *MemoryMapping) Lookup(_ context.Context, key contracts.MappingKey) (contracts.Value, bool, error) {
	if m == nil || m.data == nil {
		return contracts.Value{}, false, nil
	}
	m.mu.RLock()
	v, ok := m.data[key]
	m.mu.RUnlock()
	if !ok {
		return contracts.Value{}, false, nil
	}
	return v, true, nil
}

// GetOrCreate atomically returns the first winner written for key.
// Concurrent callers racing on the same key all observe the same
// winner, even if their candidate values differ. Context cancellation
// is honored as close-to-the-call as possible (mapping-local operations
// never block indefinitely).
func (m *MemoryMapping) GetOrCreate(ctx context.Context, key contracts.MappingKey, candidate contracts.Value) (contracts.Value, error) {
	if err := ctx.Err(); err != nil {
		return contracts.Value{}, err
	}
	if m == nil || m.data == nil {
		return contracts.Value{}, contracts.ErrClosed
	}
	// Fast read path.
	m.mu.RLock()
	if v, ok := m.data[key]; ok {
		m.mu.RUnlock()
		return v, nil
	}
	m.mu.RUnlock()

	if err := ctx.Err(); err != nil {
		return contracts.Value{}, err
	}

	// Slow path with double-checked locking.
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.data[key]; ok {
		return v, nil
	}
	m.data[key] = candidate
	return candidate, nil
}

// Len returns the number of stored entries. Intended for test
// assertions only; production code must not depend on store size.
func (m *MemoryMapping) Len() int {
	if m == nil || m.data == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.data)
}
