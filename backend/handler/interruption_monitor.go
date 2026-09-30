package handler

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// ---------------------------------------------------------------------------
// REST handlers (dashboard + auto-discovery). Registered under the auth group,
// mirroring /api/proxy-errors.
// ---------------------------------------------------------------------------

// GetInterruptions returns recent session-interruption events with optional
// config / dimension / window filters. GET /api/interruptions
func (h *Handler) GetInterruptions(c *gin.Context) {
	configID := c.Query("config_id")
	dimension := c.Query("dimension")
	windowMin, _ := strconv.Atoi(c.DefaultQuery("window", "30"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	if windowMin <= 0 {
		windowMin = 30
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	events, err := database.GetRecentInterruptions(limit, configID, dimension)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query interruptions: " + err.Error()})
		return
	}
	if events == nil {
		events = []database.SessionInterruption{}
	}

	// Trim to the requested window in Go (created_at is RFC3339 UTC, comparable).
	since := time.Now().UTC().Add(-time.Duration(windowMin) * time.Minute)
	filtered := make([]database.SessionInterruption, 0, len(events))
	for _, e := range events {
		if t, err := time.Parse(time.RFC3339, e.CreatedAt); err == nil && t.Before(since) {
			continue
		}
		filtered = append(filtered, e)
	}

	c.JSON(http.StatusOK, gin.H{"interruptions": filtered, "count": len(filtered), "window_min": windowMin})
}

// GetInterruptionStats returns interruption counts grouped by cause/dimension
// (optionally per config). GET /api/interruptions/stats
func (h *Handler) GetInterruptionStats(c *gin.Context) {
	configID := c.Query("config_id")
	windowMin, _ := strconv.Atoi(c.DefaultQuery("window", "15"))
	byConfig := c.DefaultQuery("by_config", "false") == "true"

	if windowMin <= 0 {
		windowMin = 15
	}
	since := time.Now().Add(-time.Duration(windowMin) * time.Minute)

	stats, err := database.GetInterruptionStats(since, configID, byConfig)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query interruption stats: " + err.Error()})
		return
	}
	if stats == nil {
		stats = []database.InterruptionStats{}
	}

	// Also surface recent flapping sessions for the dashboard.
	flapping, err := database.GetFlappingSessions(since, 3)
	if err != nil {
		flapping = []database.FlappingSession{}
	}
	if flapping == nil {
		flapping = []database.FlappingSession{}
	}

	c.JSON(http.StatusOK, gin.H{
		"stats":    stats,
		"flapping": flapping,
		"since":    since.UTC().Format(time.RFC3339),
		"window_min": windowMin,
	})
}

// ---------------------------------------------------------------------------
// InterruptionMonitor — near-real-time (1-min) threshold scan. LOG-ONLY.
//
// Alerting / self-heal (desktop + webhook + gated restart) is deliberately the
// shell script + systemd timer's job (scripts/monitor-interruptions.sh), which
// runs every 2 min and is the single notification source. Keeping this Go
// monitor log-only avoids two engines double-paging while still surfacing
// critical crossings in the journal promptly.
// ---------------------------------------------------------------------------

// InterruptionThresholds holds the warn/critical cutoffs. All are rate/ratio
// based, not raw counts, so they stay meaningful as traffic scales.
type InterruptionThresholds struct {
	CheckInterval            time.Duration
	InfraGlobal5mWarn        int // ≥ 8 infra interruptions globally / 5 min
	InfraGlobal5mCritical    int // ≥ 25 / 5 min
	InfraPerConfig15mWarn    int // ≥ 5 / 15 min per config
	InfraPerConfig15mCrit    int // ≥ 15 / 15 min per config
	Ratio15mWarn             float64 // > 0.15 infra/(success+infra)
	Ratio15mCritical         float64 // > 0.40 sustained 2 consecutive checks
	Flapping1hWarn           int // ≥ 4 interruptions on one session / 1h
	Flapping1hCritical       int // ≥ 8
	UpstreamStall15mWarn     int // ≥ 10 upstream_stall globally / 15 min
	UpstreamStall15mCritical int // ≥ 30 / 15 min
}

// DefaultInterruptionThresholds returns sane defaults.
func DefaultInterruptionThresholds() InterruptionThresholds {
	return InterruptionThresholds{
		CheckInterval:            1 * time.Minute,
		InfraGlobal5mWarn:        8,
		InfraGlobal5mCritical:    25,
		InfraPerConfig15mWarn:    5,
		InfraPerConfig15mCrit:    15,
		Ratio15mWarn:             0.15,
		Ratio15mCritical:         0.40,
		Flapping1hWarn:           4,
		Flapping1hCritical:       8,
		UpstreamStall15mWarn:     10,
		UpstreamStall15mCritical: 30,
	}
}

// InterruptionMonitor is a log-only background scanner.
type InterruptionMonitor struct {
	thresholds InterruptionThresholds
	stopChan   chan struct{}
	lastRatioCritical bool
}

// NewInterruptionMonitor creates the monitor.
func NewInterruptionMonitor(thresholds InterruptionThresholds) *InterruptionMonitor {
	return &InterruptionMonitor{
		thresholds: thresholds,
		stopChan:   make(chan struct{}),
	}
}

// Start runs the periodic scan until ctx is canceled or Stop is called.
func (m *InterruptionMonitor) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(m.thresholds.CheckInterval)
		defer ticker.Stop()
		m.check()
		for {
			select {
			case <-ctx.Done():
				return
			case <-m.stopChan:
				return
			case <-ticker.C:
				m.check()
			}
		}
	}()
	log.Printf("InterruptionMonitor started (interval=%s)", m.thresholds.CheckInterval)
}

// Stop halts the monitor.
func (m *InterruptionMonitor) Stop() {
	select {
	case <-m.stopChan:
	default:
		close(m.stopChan)
	}
}

// check evaluates all threshold signals and logs warn/critical crossings.
func (m *InterruptionMonitor) check() {
	now := time.Now()

	// 1. Infrastructure global rate over 5 min.
	if c, err := database.GetInterruptionCount(now.Add(-5*time.Minute), database.DimensionInfrastructure, ""); err == nil {
		if c >= m.thresholds.InfraGlobal5mCritical {
			log.Printf("[interruption CRITICAL] infra interruptions global ≥ %d / 5min (critical %d)", c, m.thresholds.InfraGlobal5mCritical)
		} else if c >= m.thresholds.InfraGlobal5mWarn {
			log.Printf("[interruption WARN] infra interruptions global ≥ %d / 5min (warn %d)", c, m.thresholds.InfraGlobal5mWarn)
		}
	}

	// 2. Per-config infrastructure rate over 15 min.
	if stats, err := database.GetInterruptionStats(now.Add(-15*time.Minute), "", true); err == nil {
		perConfig := map[string]int{}
		for _, s := range stats {
			if s.Dimension == database.DimensionInfrastructure {
				key := s.ConfigName
				if key == "" {
					key = s.ConfigID
				}
				perConfig[key] += s.Count
			}
		}
		for name, count := range perConfig {
			if count >= m.thresholds.InfraPerConfig15mCrit {
				log.Printf("[interruption CRITICAL] infra rate config=%q ≥ %d / 15min (critical %d)", name, count, m.thresholds.InfraPerConfig15mCrit)
			} else if count >= m.thresholds.InfraPerConfig15mWarn {
				log.Printf("[interruption WARN] infra rate config=%q ≥ %d / 15min (warn %d)", name, count, m.thresholds.InfraPerConfig15mWarn)
			}
		}
	}

	// 3. Disruption ratio infra/(success+infra) over 15 min (needs request_logs).
	if ratio, ok := m.disruptionRatio(now.Add(-15 * time.Minute)); ok {
		if ratio > m.thresholds.Ratio15mCritical {
			if m.lastRatioCritical {
				log.Printf("[interruption CRITICAL] disruption ratio %.2f sustained across 2 checks (> %.2f)", ratio, m.thresholds.Ratio15mCritical)
			} else {
				log.Printf("[interruption WARN] disruption ratio %.2f reached critical threshold (%.2f); confirming on next check", ratio, m.thresholds.Ratio15mCritical)
			}
			m.lastRatioCritical = true
		} else {
			m.lastRatioCritical = false
			if ratio > m.thresholds.Ratio15mWarn {
				log.Printf("[interruption WARN] disruption ratio %.2f (> %.2f warn)", ratio, m.thresholds.Ratio15mWarn)
			}
		}
	}

	// 4. Flapping sessions (any cause) over 1 h.
	if flapping, err := database.GetFlappingSessions(now.Add(-time.Hour), m.thresholds.Flapping1hWarn); err == nil {
		for _, f := range flapping {
			if f.Count >= m.thresholds.Flapping1hCritical {
				log.Printf("[interruption CRITICAL] flapping session=%s × %d / 1h (critical %d)", f.SessionID, f.Count, m.thresholds.Flapping1hCritical)
			} else {
				log.Printf("[interruption WARN] flapping session=%s × %d / 1h (warn %d)", f.SessionID, f.Count, m.thresholds.Flapping1hWarn)
			}
		}
	}

	// 5. Upstream_stall alone over 15 min: covered by the per-config infra rate
	// above and by the shell script's stall-mix analysis for the restart gate.
}

// disruptionRatio computes infra/(success+infra) over the window. Returns ok=false
// if there is too little traffic to be meaningful.
func (m *InterruptionMonitor) disruptionRatio(since time.Time) (float64, bool) {
	infra, err := database.GetInterruptionCount(since, database.DimensionInfrastructure, "")
	if err != nil {
		return 0, false
	}
	success, err := m.successCountSince(since)
	if err != nil {
		return 0, false
	}
	denom := success + infra
	if denom < 10 {
		return 0, false // not enough traffic to judge
	}
	return float64(infra) / float64(denom), true
}

// successCountSince returns the number of successful requests logged since t.
func (m *InterruptionMonitor) successCountSince(t time.Time) (int, error) {
	var count int
	err := database.DB.QueryRow(
		"SELECT COUNT(*) FROM request_logs WHERE status = 'success' AND created_at >= ?",
		t.UTC().Format(time.RFC3339),
	).Scan(&count)
	return count, err
}
