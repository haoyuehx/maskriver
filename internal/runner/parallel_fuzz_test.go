package runner

import (
	"context"
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// FuzzMaskBatchDeterminism exercises arbitrary binary-to-text encoding and
// deterministic replacement under a varying worker count using only
// synthetic domains. It does not fuzz real personal information.
func FuzzMaskBatchDeterminism(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4})
	f.Add([]byte("synthetic-only"))
	f.Add([]byte{255, 0, 128, 7})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 128 {
			t.Skip()
		}
		engine, col, _ := syntheticBatch(t, 0)
		sample := "test-" + hex.EncodeToString(data) + "@example.invalid"
		value, err := contracts.NewValue(contracts.Text, sample)
		if err != nil {
			t.Fatal("synthetic text constructor rejected encoded bytes")
		}
		original := []contracts.Value{value, value, value, value}
		serial, err := engine.maskBatch(context.Background(), original, col, 1)
		if err != nil {
			t.Fatal("serial mask failed for synthetic input")
		}
		workers := 2
		if len(data) > 0 {
			workers = 2 + int(data[0]%3)
		}
		parallel, err := engine.maskBatch(context.Background(), original, col, workers)
		if err != nil || !reflect.DeepEqual(serial, parallel) {
			t.Fatal("worker ordering or encoding invariant broken")
		}
		for _, masked := range parallel {
			if masked == value || masked.IsNull() || masked.Kind() != contracts.Text {
				t.Fatal("masked output violated typed input contract")
			}
		}
	})
}
