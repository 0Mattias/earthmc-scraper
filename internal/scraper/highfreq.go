package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/0Mattias/earthmc-scraper/internal/api"
)

// HighFreq scrapes online player status and map coordinates every interval.
type HighFreq struct {
	client             *api.Client
	pool               *pgxpool.Pool
	interval           time.Duration
	running            sync.Mutex
	lastPartitionCheck time.Time
}

// activityRow represents a single player activity record.
type activityRow struct {
	PlayerUUID string
	PlayerName string
	IsOnline   bool
	IsVisible  bool
	X, Y, Z    *int
	Yaw        *int
	World      *string
}

// NewHighFreq creates a new high-frequency scraper.
func NewHighFreq(client *api.Client, pool *pgxpool.Pool, interval time.Duration) *HighFreq {
	return &HighFreq{
		client:   client,
		pool:     pool,
		interval: interval,
	}
}

// ensurePartitions calls the DB function to create upcoming hourly partitions.
// Only runs once every 30 minutes to avoid unnecessary overhead.
func (h *HighFreq) ensurePartitions(ctx context.Context) {
	if time.Since(h.lastPartitionCheck) < 30*time.Minute {
		return
	}
	_, err := h.pool.Exec(ctx, "SELECT create_activity_partitions(NOW(), 48)")
	if err != nil {
		slog.Error("failed to create partitions", "error", err)
		return
	}
	h.lastPartitionCheck = time.Now()
	slog.Info("ensured hourly partitions exist for next 48 hours")
}

// Run starts the high-frequency scrape loop. Blocks until context is cancelled.
func (h *HighFreq) Run(ctx context.Context) {
	slog.Info("high-freq scraper started", "interval", h.interval)
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	// Run immediately on start
	h.tick(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("high-freq scraper stopped")
			return
		case <-ticker.C:
			h.tick(ctx)
		}
	}
}

func (h *HighFreq) tick(ctx context.Context) {
	// Skip if previous tick is still running
	if !h.running.TryLock() {
		slog.Warn("high-freq tick skipped: previous still running")
		return
	}
	defer h.running.Unlock()

	// Ensure hourly partitions exist ahead of current time
	h.ensurePartitions(ctx)

	start := time.Now()
	snapshotTS := start

	// Fetch online players and map positions concurrently
	var (
		onlineResp *api.OnlineResponse
		mapResp    *api.MapPlayersResponse
		onlineErr  error
		mapErr     error
		wg         sync.WaitGroup
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		onlineResp, onlineErr = h.client.GetOnline(ctx)
	}()
	go func() {
		defer wg.Done()
		mapResp, mapErr = h.client.GetMapPlayers(ctx)
	}()
	wg.Wait()

	if onlineErr != nil {
		slog.Error("high-freq: failed to fetch online players", "error", onlineErr)
		return
	}
	if mapErr != nil {
		slog.Warn("high-freq: failed to fetch map players, proceeding without coords", "error", mapErr)
		mapResp = &api.MapPlayersResponse{}
	}

	// Build map of visible players by UUID (normalize UUID: remove dashes)
	visibleMap := make(map[string]*api.MapPlayer, len(mapResp.Players))
	for i := range mapResp.Players {
		p := &mapResp.Players[i]
		normalizedUUID := normalizeUUID(p.UUID)
		visibleMap[normalizedUUID] = p
	}

	// Build activity rows
	rows := make([]activityRow, 0, len(onlineResp.Players))
	for _, op := range onlineResp.Players {
		normalizedUUID := normalizeUUID(op.UUID)
		row := activityRow{
			PlayerUUID: op.UUID,
			PlayerName: op.Name,
			IsOnline:   true,
			IsVisible:  false,
		}

		if mp, ok := visibleMap[normalizedUUID]; ok {
			row.IsVisible = true
			row.X = &mp.X
			row.Y = &mp.Y
			row.Z = &mp.Z
			row.Yaw = &mp.Yaw
			row.World = &mp.World
		}

		rows = append(rows, row)
	}

	if len(rows) == 0 {
		slog.Debug("high-freq: no online players")
		return
	}

	// Batch insert using a single multi-value INSERT for speed
	if err := h.insertActivity(ctx, snapshotTS, rows); err != nil {
		slog.Error("high-freq: insert activity failed", "error", err)
		return
	}

	// Upsert dimension table
	if err := h.upsertPlayers(ctx, snapshotTS, rows); err != nil {
		slog.Error("high-freq: upsert players failed", "error", err)
	}

	slog.Info("high-freq tick complete",
		"online", onlineResp.Count,
		"visible", len(visibleMap),
		"inserted", len(rows),
		"duration", time.Since(start).Round(time.Millisecond),
	)
}

func (h *HighFreq) insertActivity(ctx context.Context, ts time.Time, rows []activityRow) error {
	// Build multi-value INSERT for maximum throughput
	var sb strings.Builder
	sb.WriteString("INSERT INTO player_activity (snapshot_ts, player_uuid, player_name, is_online, is_visible, x, y, z, yaw, world) VALUES ")

	args := make([]interface{}, 0, len(rows)*10)
	for i, r := range rows {
		if i > 0 {
			sb.WriteString(",")
		}
		base := i * 10
		sb.WriteString(fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10))
		args = append(args, ts, r.PlayerUUID, r.PlayerName, r.IsOnline, r.IsVisible, r.X, r.Y, r.Z, r.Yaw, r.World)
	}

	_, err := h.pool.Exec(ctx, sb.String(), args...)
	return err
}

func (h *HighFreq) upsertPlayers(ctx context.Context, ts time.Time, rows []activityRow) error {
	var sb strings.Builder
	sb.WriteString("INSERT INTO players (uuid, name, first_seen, last_seen) VALUES ")

	args := make([]interface{}, 0, len(rows)*4)
	for i, r := range rows {
		if i > 0 {
			sb.WriteString(",")
		}
		base := i * 4
		sb.WriteString(fmt.Sprintf("($%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4))
		args = append(args, r.PlayerUUID, r.PlayerName, ts, ts)
	}

	sb.WriteString(" ON CONFLICT (uuid) DO UPDATE SET name = EXCLUDED.name, last_seen = EXCLUDED.last_seen")

	_, err := h.pool.Exec(ctx, sb.String(), args...)
	return err
}

// normalizeUUID strips dashes from a UUID for comparison.
// The map API returns UUIDs without dashes, the /online API returns them with dashes.
func normalizeUUID(uuid string) string {
	return strings.ReplaceAll(uuid, "-", "")
}

// ============================================================
// Typed detail extraction helpers for parsing raw JSON
// ============================================================

func extractNameUUID(raw json.RawMessage) (name, uuid string, err error) {
	var entry struct {
		Name string `json:"name"`
		UUID string `json:"uuid"`
	}
	if err := json.Unmarshal(raw, &entry); err != nil {
		return "", "", err
	}
	return entry.Name, entry.UUID, nil
}
