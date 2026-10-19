package runner

import (
	"encoding/json"
	"strings"
	"testing"
)

func validPhase3ValidationSamples() []phase3Trial {
	samples := make([]phase3Trial, 0, 4)
	for order, batch := range []int{16, 64, 128, 256} {
		batches := (256 + batch - 1) / batch
		windows := make([]int64, batches)
		rowsPerReceipt := make([]int64, batches)
		statements := make([]int, batches)
		updates := make([]int, batches)
		other := make([]int, batches)
		beginDriver := make([]int64, batches)
		commitDriver := make([]int64, batches)
		updateExec := make([]int64, batches)
		otherSQL := make([]int64, batches)
		for i := 0; i < batches; i++ {
			rows := batch
			if i == batches-1 && 256%batch != 0 {
				rows = 256 % batch
			}
			windows[i] = 10
			rowsPerReceipt[i] = int64(rows)
			statements[i] = rows + 7
			updates[i] = rows
			other[i] = 5
		}
		windowSum := int64(batches * 10)
		samples = append(samples, phase3Trial{
			Order: order + 1, Trial: 1, BatchSize: batch, Rows: 256, Workers: 4,
			FixtureNS: 1, WorkflowNS: 2000, ApplyNS: 1000, RowsPerSecond: 256e6,
			BeginAttempts: batches, BeginSucceeded: batches,
			CommitAttempts: batches, CommitReturnedSuccess: batches,
			BeginWallNS: 1, CommitWallNS: 1, UpdateCallWallNS: 1,
			Statements: 256 + batches*7, UpdateStatements: 256,
			BatchTransactionNS: windows, BatchTransactionSumNS: windowSum,
			ApplyUnassignedNS: 1000 - windowSum, ReceiptRows: rowsPerReceipt,
			JournalMode: "delete", Synchronous: "2",
			BatchStatementCounts: statements, BatchUpdateCounts: updates, BatchOtherCounts: other,
			BatchBeginDriverNS: beginDriver, BatchCommitDriverNS: commitDriver,
			BatchUpdateExecNS: updateExec, BatchOtherSQLCallNS: otherSQL,
			Receipts: batches, RowsCommitted: 256, Complete: true, ValidatePassed: true,
		})
	}
	return samples
}

func TestValidatePhase3TrialsRejectsInconsistentThroughput(t *testing.T) {
	samples := validPhase3ValidationSamples()
	if err := validatePhase3Trials(samples, 1); err != nil {
		t.Fatalf("valid fixture rejected: %v", err)
	}
	samples[0].RowsPerSecond++
	if err := validatePhase3Trials(samples, 1); err == nil || !strings.Contains(err.Error(), "throughput") {
		t.Fatalf("inconsistent throughput accepted or wrong error: %v", err)
	}
}

func TestValidatePhase3TrialsRejectsNegativeDurations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*phase3Trial)
	}{
		{"driver commit", func(s *phase3Trial) { s.DriverCommitNS = -1 }},
		{"batch other SQL", func(s *phase3Trial) { s.BatchOtherSQLCallNS[0] = -1 }},
		{"batch window", func(s *phase3Trial) { s.BatchTransactionNS[0] = -1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			samples := validPhase3ValidationSamples()
			tc.mutate(&samples[0])
			if err := validatePhase3Trials(samples, 1); err == nil || !strings.Contains(err.Error(), "negative") {
				t.Fatalf("negative duration accepted or wrong error: %v", err)
			}
		})
	}
}

func TestPhase3EnvironmentValidationUsesMeasurementProvenance(t *testing.T) {
	samples := validPhase3ValidationSamples()
	raw := []byte(phase3Environment(samples, 1))
	if err := validatePhase3Environment(raw, samples, 1); err != nil {
		t.Fatalf("valid measurement metadata rejected: %v", err)
	}
	// A later checkout SHA is not substituted for the code SHA that produced the
	// measurement; any valid recorded code SHA remains provenance.
	var environment phase3EnvironmentMetadata
	if err := json.Unmarshal(raw, &environment); err != nil {
		t.Fatal(err)
	}
	environment.HeadSHA = strings.Repeat("a", 40)
	updated, err := json.Marshal(environment)
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePhase3Environment(updated, samples, 1); err != nil {
		t.Fatalf("historical code SHA rejected: %v", err)
	}
	environment.Samples++
	updated, _ = json.Marshal(environment)
	if err := validatePhase3Environment(updated, samples, 1); err == nil {
		t.Fatal("inconsistent sample-count metadata accepted")
	}
}
