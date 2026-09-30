package database

import (
	"strings"
	"testing"
	"time"
)

// ensureTestDB guarantees a live global DB for this test. The package's shared
// TestMain DB can be poisoned by other tests (models_mapping_test.go closes the
// global DB with its own defer), so re-open a fresh in-memory DB when needed.
// Running migrations through createTables creates session_interruptions.
func ensureTestDB(t *testing.T) {
	t.Helper()
	if DB != nil {
		if err := DB.Ping(); err == nil {
			return
		}
	}
	if _, err := InitTestDB(); err != nil {
		t.Fatalf("ensureTestDB InitTestDB: %v", err)
	}
}


func backdateInterruption(t *testing.T, id int64, ago time.Duration) {
	t.Helper()
	_, err := DB.Exec("UPDATE session_interruptions SET created_at = ? WHERE id = ?",
		time.Now().UTC().Add(-ago).Format(time.RFC3339), id)
	if err != nil {
		t.Fatalf("backdate failed: %v", err)
	}
}

// TestLogInterruptionSyncInsertAndTruncate verifies a basic insert and the
// maxInterruptionDetailLen truncation of the detail column.
func TestLogInterruptionSyncInsertAndTruncate(t *testing.T) {
	ensureTestDB(t)
	// clean
	_, _ = DB.Exec("DELETE FROM session_interruptions")

	long := strings.Repeat("x", maxInterruptionDetailLen+100)
	e := &SessionInterruption{
		SessionID: "sess-1", RequestID: "req-1", ConfigID: "cfg-1", ConfigName: "opencode-cc",
		Model: "claude-sonnet", InterruptionCause: "upstream_stall",
		Dimension: DimensionInfrastructure, Stage: StageStreaming, Detail: long, DurationMs: 12000,
	}
	if err := LogInterruptionSync(e); err != nil {
		t.Fatalf("LogInterruptionSync failed: %v", err)
	}

	rows, err := GetRecentInterruptions(10, "", "")
	if err != nil {
		t.Fatalf("GetRecentInterruptions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	got := rows[0]
	if got.InterruptionCause != "upstream_stall" || got.Dimension != DimensionInfrastructure {
		t.Errorf("unexpected classify: cause=%s dim=%s", got.InterruptionCause, got.Dimension)
	}
	if len(got.Detail) > maxInterruptionDetailLen {
		t.Errorf("detail not truncated: len=%d", len(got.Detail))
	}
	if got.SessionID != "sess-1" {
		t.Errorf("session_id mismatch: %s", got.SessionID)
	}
}

// TestInterruptionStatsWindowAndGrouping seeds in-window and out-of-window rows and
// asserts GetInterruptionStats counts windowed rows and groups by cause/dimension/config.
func TestInterruptionStatsWindowAndGrouping(t *testing.T) {
	ensureTestDB(t)
	_, _ = DB.Exec("DELETE FROM session_interruptions")

	seed := []*SessionInterruption{
		// 3 in-window infrastructure upstream_stall, config A
		{SessionID: "s1", ConfigID: "cfgA", ConfigName: "cc-a", Model: "m", InterruptionCause: "upstream_stall", Dimension: DimensionInfrastructure, Stage: StageStreaming},
		{SessionID: "s2", ConfigID: "cfgA", ConfigName: "cc-a", Model: "m", InterruptionCause: "upstream_stall", Dimension: DimensionInfrastructure, Stage: StageStreaming},
		{SessionID: "s3", ConfigID: "cfgA", ConfigName: "cc-a", Model: "m", InterruptionCause: "upstream_stall", Dimension: DimensionInfrastructure, Stage: StageStreaming},
		// 2 in-window subjective client_disconnect, config A
		{SessionID: "s4", ConfigID: "cfgA", ConfigName: "cc-a", Model: "m", InterruptionCause: "client_disconnect", Dimension: DimensionSubjective, Stage: StageStreaming},
		{SessionID: "s5", ConfigID: "cfgA", ConfigName: "cc-a", Model: "m", InterruptionCause: "client_disconnect", Dimension: DimensionSubjective, Stage: StageStreaming},
		// 2 in-window infrastructure upstream_stall, config B
		{SessionID: "s6", ConfigID: "cfgB", ConfigName: "cc-b", Model: "m", InterruptionCause: "upstream_stall", Dimension: DimensionInfrastructure, Stage: StageStreaming},
		{SessionID: "s7", ConfigID: "cfgB", ConfigName: "cc-b", Model: "m", InterruptionCause: "upstream_stall", Dimension: DimensionInfrastructure, Stage: StageStreaming},
	}
	for _, e := range seed {
		// use direct insert so we can control created_at; write old timestamps for some
		if e.SessionID == "s7" {
			// out-of-window: backdate below the 5-min window
			if err := LogInterruptionSync(e); err != nil {
				t.Fatalf("seed insert failed: %v", err)
			}
			// find its id and backdate
			var id int64
			if err := DB.QueryRow("SELECT id FROM session_interruptions WHERE session_id='s7'").Scan(&id); err != nil {
				t.Fatalf("locate row: %v", err)
			}
			backdateInterruption(t, id, 10*time.Minute+time.Second)
			continue
		}
		if err := LogInterruptionSync(e); err != nil {
			t.Fatalf("seed insert failed: %v", err)
		}
	}

	// Global aggregate over 5-min window: s7 excluded -> 4 infrastructure, 2 subjective
	stats, err := GetInterruptionStats(time.Now().Add(-5*time.Minute), "", false)
	if err != nil {
		t.Fatalf("GetInterruptionStats failed: %v", err)
	}
	total := 0
	var infra, subj int
	for _, s := range stats {
		total += s.Count
		if s.Dimension == DimensionInfrastructure {
			infra += s.Count
		}
		if s.Dimension == DimensionSubjective {
			subj += s.Count
		}
	}
	if total != 6 {
		t.Errorf("global window total = %d, want 6", total)
	}
	if infra != 4 {
		t.Errorf("global infra count = %d, want 4", infra)
	}
	if subj != 2 {
		t.Errorf("global subjective count = %d, want 2", subj)
	}

	// Per-config aggregation
	perCfg, err := GetInterruptionStats(time.Now().Add(-5*time.Minute), "", true)
	if err != nil {
		t.Fatalf("GetInterruptionStats(byConfig) failed: %v", err)
	}
	cfgACount := 0
	cfgBCount := 0
	for _, s := range perCfg {
		switch s.ConfigID {
		case "cfgA":
			cfgACount += s.Count
		case "cfgB":
			cfgBCount += s.Count
		}
	}
	if cfgACount != 5 {
		t.Errorf("cfgA count = %d, want 5", cfgACount)
	}
	if cfgBCount != 1 {
		t.Errorf("cfgB count = %d, want 1 (s7 out-of-window)", cfgBCount)
	}

	// GetInterruptionCount filtered by dimension + config
	c, err := GetInterruptionCount(time.Now().Add(-5*time.Minute), DimensionInfrastructure, "cfgA")
	if err != nil {
		t.Fatalf("GetInterruptionCount failed: %v", err)
	}
	if c != 3 {
		t.Errorf("GetInterruptionCount(cfgA,infra) = %d, want 3", c)
	}
}

// TestGetFlappingSessions groups repeated interruptions on the same session.
func TestGetFlappingSessions(t *testing.T) {
	ensureTestDB(t)
	_, _ = DB.Exec("DELETE FROM session_interruptions")

	for i := 0; i < 5; i++ {
		if err := LogInterruptionSync(&SessionInterruption{
			SessionID: "flap-1", ConfigID: "cfgA", ConfigName: "cc-a", Model: "m",
			InterruptionCause: "client_disconnect", Dimension: DimensionSubjective, Stage: StageStreaming,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := LogInterruptionSync(&SessionInterruption{
			SessionID: "quiet-1", ConfigID: "cfgA", ConfigName: "cc-a", Model: "m",
			InterruptionCause: "upstream_stall", Dimension: DimensionInfrastructure, Stage: StageStreaming,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	sessions, err := GetFlappingSessions(time.Now().Add(-time.Hour), 3)
	if err != nil {
		t.Fatalf("GetFlappingSessions failed: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 flapping session, got %d", len(sessions))
	}
	if sessions[0].SessionID != "flap-1" || sessions[0].Count != 5 {
		t.Errorf("unexpected flapping row: %+v", sessions[0])
	}
}

// TestCleanupOldSessionInterruptions deletes rows older than the retention window.
func TestCleanupOldSessionInterruptions(t *testing.T) {
	ensureTestDB(t)
	_, _ = DB.Exec("DELETE FROM session_interruptions")

	if err := LogInterruptionSync(&SessionInterruption{
		SessionID: "old-1", ConfigID: "cfgA", InterruptionCause: "upstream_stall", Dimension: DimensionInfrastructure, Stage: StageStreaming,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := LogInterruptionSync(&SessionInterruption{
		SessionID: "new-1", ConfigID: "cfgA", InterruptionCause: "upstream_stall", Dimension: DimensionInfrastructure, Stage: StageStreaming,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var oldID int64
	if err := DB.QueryRow("SELECT id FROM session_interruptions WHERE session_id='old-1'").Scan(&oldID); err != nil {
		t.Fatalf("locate old: %v", err)
	}
	backdateInterruption(t, oldID, 15*24*time.Hour)

	n, err := CleanupOldSessionInterruptions(7 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if n != 1 {
		t.Errorf("cleanup deleted %d rows, want 1", n)
	}
	rows, _ := GetRecentInterruptions(10, "", "")
	if len(rows) != 1 || rows[0].SessionID != "new-1" {
		t.Errorf("expected only new-1 to remain, got %d rows", len(rows))
	}
}