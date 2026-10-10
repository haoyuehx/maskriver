package mask

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

func testMappingKey() contracts.MappingKey {
	var fingerprint [32]byte
	fingerprint[0] = 1
	return contracts.MappingKey{
		Strategy:             contracts.StrategyRef{ID: "redact", Version: strategyVersion},
		Scope:                "synthetic-scope",
		NormalizationVersion: normalizationVersion,
		KeyID:                "synthetic-key-id",
		Fingerprint:          fingerprint,
	}
}

func TestMemoryMappingLookupMissDoesNotWrite(t *testing.T) {
	mapping := NewMemoryMapping()
	key := testMappingKey()
	if value, ok, err := mapping.Lookup(context.Background(), key); err != nil || ok || !value.IsNull() {
		t.Fatal("lookup miss returned a value")
	}
	if len(mapping.entries) != 0 {
		t.Fatal("lookup miss mutated the mapping")
	}
	candidate := testValue(t, contracts.Text, "masked-candidate")
	got, err := mapping.GetOrCreate(context.Background(), key, candidate)
	if err != nil || got != candidate {
		t.Fatal("candidate was not stored")
	}
	got, ok, err := mapping.Lookup(context.Background(), key)
	if err != nil || !ok || got != candidate {
		t.Fatal("stored candidate was not found")
	}
}

func TestMemoryMappingAtomicSingleWinner(t *testing.T) {
	mapping := NewMemoryMapping()
	key := testMappingKey()
	const workers = 64
	start := make(chan struct{})
	results := make(chan contracts.Value, workers)
	errorsSeen := make(chan error, workers)
	var group sync.WaitGroup
	for i := 0; i < workers; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			<-start
			candidate, err := contracts.NewValue(contracts.Text, fmt.Sprintf("masked-%02d", i))
			if err != nil {
				errorsSeen <- err
				return
			}
			winner, err := mapping.GetOrCreate(context.Background(), key, candidate)
			if err != nil {
				errorsSeen <- err
				return
			}
			results <- winner
		}(i)
	}
	close(start)
	group.Wait()
	close(results)
	close(errorsSeen)
	for err := range errorsSeen {
		t.Fatal(err)
	}
	var winner contracts.Value
	for result := range results {
		if winner.IsNull() {
			winner = result
		}
		if result != winner {
			t.Fatal("concurrent callers observed different winners")
		}
	}
	if len(mapping.entries) != 1 {
		t.Fatal("concurrent get-or-create stored more than one entry")
	}
}

func TestMemoryMappingValidationCancellationAndRedaction(t *testing.T) {
	mapping := NewMemoryMapping()
	key := testMappingKey()
	candidate := testValue(t, contracts.Text, "masked-candidate")
	if _, _, err := mapping.Lookup(nil, key); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatal("nil context accepted")
	}
	if _, err := mapping.GetOrCreate(context.Background(), contracts.MappingKey{}, candidate); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatal("empty mapping key accepted")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := mapping.Lookup(cancelled, key); !errors.Is(err, context.Canceled) {
		t.Fatal("lookup cancellation not propagated")
	}
	if _, err := mapping.GetOrCreate(cancelled, key, candidate); !errors.Is(err, context.Canceled) {
		t.Fatal("get-or-create cancellation not propagated")
	}
	secret := "synthetic-original-must-not-appear"
	if strings.Contains(fmt.Sprintf("%#v", mapping), secret) || fmt.Sprint(mapping) != "[REDACTED]" {
		t.Fatal("mapping formatting was not redacted")
	}
}
