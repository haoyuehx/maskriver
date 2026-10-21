package runner

// Resource-budget enforcement for the streaming path.
//
// The contract's ResourceBudget is a set of limits, not a promise that every
// limit can be enforced before allocation. This file makes the enforceable part
// explicit:
//
//   - MaxRowBytes and MaxColumns/MaxTables: checked before any masking or write
//     for the page just read. The reader has already materialised the page, so
//     this is a post-materialisation check, not a pre-allocation guard. The
//     retained working set is still bounded by the page size, not by row count.
//   - MaxProcessBytes: a structural admission check over the worst-case page
//     working set plus a fixed runtime allowance. It is not an OS-level RSS cap
//     and cannot be one without platform resource isolation.
//   - MaxDiskBytes: an admission check before each transaction plus a
//     post-commit check. SQLite can still grow between the check and the commit,
//     and rollback journal/temp space is not fully attributable; a commit-time
//     ENOSPC remains CommitUnknown by design.
//   - MaxQueue: only 0 (synchronous delivery) is implemented.
//
// Every violation fails closed before the affected write. Already committed
// batches remain reported and the run is never reported complete.

import (
	"context"
	"errors"
	"os"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

const (
	// processBaselineBytes is a fixed allowance for the Go runtime, SQLite
	// driver state, schema metadata and plan tables. It is deliberately a
	// coarse structural constant, not a measured hard cap.
	processBaselineBytes int64 = 16 << 20
	// pageWorkingSetFactor covers source page, target page, RowChange values and
	// the masked replacements alive at the same time.
	pageWorkingSetFactor int64 = 4
	// diskWorkingSetFactor conservatively covers data pages, B-tree overhead and
	// the rollback journal for one batch.
	diskWorkingSetFactor int64 = 3
)

// rowPayloadBytes returns the sum of the payload bytes of one projected row.
// NULL values contribute zero. The metric is the projected row (primary key plus
// planned columns), which is all this path reads.
func rowPayloadBytes(row contracts.Row) int64 {
	var total int64
	for _, value := range row {
		total += int64(len(value.Payload()))
	}
	return total
}

// pagePayloadBytes sums the source page and checks the per-row bound. A row over
// the declared bound refuses the page before masking or writing.
func pagePayloadBytes(rows []contracts.Row, maxRowBytes int64) (int64, error) {
	var total int64
	for _, row := range rows {
		size := rowPayloadBytes(row)
		if size > maxRowBytes {
			return 0, contracts.ErrUnsafe
		}
		total += size
	}
	return total, nil
}

// changePayloadBytes is the projected write row: primary key plus every
// replacement value. A strategy may produce a longer value than the original,
// so MaxRowBytes must cover the replacement too; otherwise a run could commit a
// row that its own validation then refuses to read.
func changePayloadBytes(change contracts.RowChange) int64 {
	var total int64
	for _, value := range change.Key {
		total += int64(len(value.Payload()))
	}
	for _, value := range change.Replacement {
		total += int64(len(value.Payload()))
	}
	return total
}

// checkProcessAdmission bounds the worst-case page working set. maxProcessBytes
// is enforced as an admission rule; it is not a promise about OS RSS.
func checkProcessAdmission(pageBytes int64, maxProcessBytes int64) error {
	if pageBytes < 0 || maxProcessBytes <= 0 {
		return contracts.ErrInvalid
	}
	required := pageBytes*pageWorkingSetFactor + processBaselineBytes
	if required < 0 || required > maxProcessBytes {
		return contracts.ErrUnsafe
	}
	return nil
}

// validateProcessBudgetCombination rejects a configuration whose worst case can
// not fit MaxProcessBytes. This runs before any I/O, so an impossible budget
// combination is refused at the start rather than after a large allocation.
func validateProcessBudgetCombination(budget contracts.ResourceBudget) error {
	if budget.MaxBatchSize <= 0 || budget.MaxRowBytes <= 0 || budget.MaxProcessBytes <= 0 {
		return contracts.ErrInvalid
	}
	perBatch := int64(budget.MaxBatchSize) * budget.MaxRowBytes
	if perBatch <= 0 {
		return contracts.ErrInvalid
	}
	required := perBatch*pageWorkingSetFactor + processBaselineBytes
	if required < 0 || required > budget.MaxProcessBytes {
		return contracts.ErrUnsafe
	}
	return nil
}

// diskUsageBytes reports the main database plus its rollback/WAL sidecars. It is
// the space this process can attribute to the target; it is not a filesystem
// quota and does not include driver temp files outside the target directory.
func diskUsageBytes(path string) (int64, error) {
	var total int64
	for _, candidate := range []string{path, path + "-journal", path + "-wal"} {
		info, err := os.Stat(candidate)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return 0, contracts.ErrDatabase
		}
		total += info.Size()
	}
	return total, nil
}

// checkDiskAdmission refuses a batch when current usage plus a conservative
// estimate of the batch's growth already exceeds the budget.
func checkDiskAdmission(path string, pageBytes int64, maxDiskBytes int64) error {
	if maxDiskBytes <= 0 || pageBytes < 0 {
		return contracts.ErrInvalid
	}
	used, err := diskUsageBytes(path)
	if err != nil {
		return err
	}
	projected := used + pageBytes*diskWorkingSetFactor
	if projected < 0 || projected > maxDiskBytes {
		return contracts.ErrUnsafe
	}
	return nil
}

// checkDiskAfterCommit fails closed when a committed batch pushed the target over
// the declared disk budget. The data is already committed and is reported as a
// failure, never retried or deleted.
func checkDiskAfterCommit(path string, maxDiskBytes int64) error {
	used, err := diskUsageBytes(path)
	if err != nil {
		return err
	}
	if used > maxDiskBytes {
		return contracts.ErrUnsafe
	}
	return nil
}

// checkPageResources applies the row, process and (pre-batch) disk admission
// rules. It is the single entry point used by the streaming table loop.
func (e *Engine) checkPageResources(ctx context.Context, rows []contracts.Row,
	pageBytes int64, budget contracts.ResourceBudget) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := checkProcessAdmission(pageBytes, budget.MaxProcessBytes); err != nil {
		return err
	}
	return checkDiskAdmission(e.cfg.Target, pageBytes, budget.MaxDiskBytes)
}
