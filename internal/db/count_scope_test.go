package db

import (
	"context"
	"errors"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// Count must reject references outside DatabaseOptions' logical dataset and
// schema allowlist before any metadata or row-count query is executed.
func TestSQLiteCountRejectsOutOfScopeTableRefs(t *testing.T) {
	ctx := context.Background()
	opts := sqliteOptions(t)
	opened, err := OpenReader(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()

	allowed := table("people")
	if count, err := opened.Count(ctx, allowed); err != nil || count != 25 {
		t.Fatalf("allowed Count: count=%d err=%v", count, err)
	}

	tests := []struct {
		name string
		ref  contracts.TableRef
	}{
		{name: "wrong dataset", ref: contracts.TableRef{
			Database: "other-dataset", Schema: "main", Table: "people",
		}},
		{name: "wrong schema", ref: contracts.TableRef{
			Database: "fixture", Schema: "other", Table: "people",
		}},
		{name: "empty table", ref: contracts.TableRef{
			Database: "fixture", Schema: "main",
		}},
		{name: "NUL in table", ref: contracts.TableRef{
			Database: "fixture", Schema: "main", Table: "people\x00other",
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := opened.Count(ctx, tc.ref); !errors.Is(err, contracts.ErrInvalid) {
				t.Fatalf("out-of-scope Count should reject with ErrInvalid, got %v", err)
			}
		})
	}
}
