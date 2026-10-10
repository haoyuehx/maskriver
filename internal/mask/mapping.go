package mask

import (
	"context"
	"sync"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

var (
	_ contracts.MappingReader = (*MemoryMapping)(nil)
	_ contracts.MappingWriter = (*MemoryMapping)(nil)
)

// MemoryMapping is a process-local, concurrency-safe mapping. It stores only
// caller-supplied HMAC fingerprint keys and masked candidates, never originals.
type MemoryMapping struct {
	mu      sync.RWMutex
	entries map[contracts.MappingKey]contracts.Value
}

// NewMemoryMapping returns an empty in-memory mapping.
func NewMemoryMapping() *MemoryMapping {
	return &MemoryMapping{entries: make(map[contracts.MappingKey]contracts.Value)}
}

func (m *MemoryMapping) Lookup(ctx context.Context, key contracts.MappingKey) (contracts.Value, bool, error) {
	if err := validateMappingCall(ctx, key); err != nil {
		return contracts.Value{}, false, err
	}
	m.mu.RLock()
	value, ok := m.entries[key]
	m.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return contracts.Value{}, false, err
	}
	return value, ok, nil
}

func (m *MemoryMapping) GetOrCreate(ctx context.Context, key contracts.MappingKey, candidate contracts.Value) (contracts.Value, error) {
	if err := validateMappingCall(ctx, key); err != nil {
		return contracts.Value{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return contracts.Value{}, err
	}
	if value, ok := m.entries[key]; ok {
		return value, nil
	}
	if m.entries == nil {
		m.entries = make(map[contracts.MappingKey]contracts.Value)
	}
	m.entries[key] = candidate
	return candidate, nil
}

func (*MemoryMapping) String() string   { return "[REDACTED]" }
func (*MemoryMapping) GoString() string { return "[REDACTED]" }

func validateMappingCall(ctx context.Context, key contracts.MappingKey) error {
	if ctx == nil || key.Strategy.ID == "" || key.Strategy.Version == "" || key.Scope == "" ||
		key.NormalizationVersion == "" || key.KeyID == "" || key.Fingerprint == ([32]byte{}) {
		return contracts.ErrInvalid
	}
	return ctx.Err()
}
