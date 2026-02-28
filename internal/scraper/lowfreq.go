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
	"golang.org/x/sync/errgroup"

	"github.com/0Mattias/earthmc-scraper/internal/api"
)

// LowFreq scrapes full server/player/town/nation data every interval.
type LowFreq struct {
	client   *api.Client
	pool     *pgxpool.Pool
	interval time.Duration
	running  sync.Mutex
}

// NewLowFreq creates a new low-frequency scraper.
func NewLowFreq(client *api.Client, pool *pgxpool.Pool, interval time.Duration) *LowFreq {
	return &LowFreq{
		client:   client,
		pool:     pool,
		interval: interval,
	}
}

// Run starts the low-frequency scrape loop. Blocks until context is cancelled.
func (l *LowFreq) Run(ctx context.Context) {
	slog.Info("low-freq scraper started", "interval", l.interval)
	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()

	// Run immediately on start
	l.tick(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("low-freq scraper stopped")
			return
		case <-ticker.C:
			l.tick(ctx)
		}
	}
}

func (l *LowFreq) tick(ctx context.Context) {
	if !l.running.TryLock() {
		slog.Warn("low-freq tick skipped: previous still running")
		return
	}
	defer l.running.Unlock()

	start := time.Now()
	snapshotTS := start

	// Run all entity scrapes concurrently with error isolation
	g, gCtx := errgroup.WithContext(ctx)

	// 1. Server status
	g.Go(func() error {
		if err := l.scrapeServer(gCtx, snapshotTS); err != nil {
			slog.Error("low-freq: server scrape failed", "error", err)
			// Don't propagate — isolate failures
		}
		return nil
	})

	// 2. Towns
	g.Go(func() error {
		if err := l.scrapeTowns(gCtx, snapshotTS); err != nil {
			slog.Error("low-freq: towns scrape failed", "error", err)
		}
		return nil
	})

	// 3. Nations
	g.Go(func() error {
		if err := l.scrapeNations(gCtx, snapshotTS); err != nil {
			slog.Error("low-freq: nations scrape failed", "error", err)
		}
		return nil
	})

	// 4. Players
	g.Go(func() error {
		if err := l.scrapePlayers(gCtx, snapshotTS); err != nil {
			slog.Error("low-freq: players scrape failed", "error", err)
		}
		return nil
	})

	_ = g.Wait()

	slog.Info("low-freq tick complete",
		"duration", time.Since(start).Round(time.Millisecond),
	)
}

// ---- Server ----

func (l *LowFreq) scrapeServer(ctx context.Context, ts time.Time) error {
	srv, err := l.client.GetServer(ctx)
	if err != nil {
		return fmt.Errorf("get server: %w", err)
	}

	_, err = l.pool.Exec(ctx, `
		INSERT INTO server_snapshots (
			snapshot_ts, version, moon_phase, has_storm, is_thundering,
			server_time, full_time, max_players, num_online_players, num_online_nomads,
			num_residents, num_nomads, num_towns, num_town_blocks, num_nations,
			num_quarters, num_cuboids, vote_party_target, vote_party_remaining
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
		ts, srv.Version, srv.MoonPhase, srv.Status.HasStorm, srv.Status.IsThundering,
		srv.Stats.Time, srv.Stats.FullTime, srv.Stats.MaxPlayers, srv.Stats.NumOnlinePlayers, srv.Stats.NumOnlineNomads,
		srv.Stats.NumResidents, srv.Stats.NumNomads, srv.Stats.NumTowns, srv.Stats.NumTownBlocks, srv.Stats.NumNations,
		srv.Stats.NumQuarters, srv.Stats.NumCuboids, srv.VoteParty.Target, srv.VoteParty.NumRemaining,
	)
	if err != nil {
		return fmt.Errorf("insert server snapshot: %w", err)
	}

	slog.Info("server snapshot saved", "online", srv.Stats.NumOnlinePlayers, "towns", srv.Stats.NumTowns, "nations", srv.Stats.NumNations)
	return nil
}

// ---- Towns ----

func (l *LowFreq) scrapeTowns(ctx context.Context, ts time.Time) error {
	// Step 1: Get town list
	townList, err := l.client.GetTownsList(ctx)
	if err != nil {
		return fmt.Errorf("get towns list: %w", err)
	}
	slog.Info("fetched town list", "count", len(townList))

	// Step 2: Fetch full details via POST
	uuids := make([]string, len(townList))
	for i, t := range townList {
		uuids[i] = t.UUID
	}

	details, err := l.client.PostTowns(ctx, uuids)
	if err != nil {
		return fmt.Errorf("post towns: %w", err)
	}
	slog.Info("fetched town details", "count", len(details))

	// Step 3: Insert snapshots and upsert dimensions
	if err := l.insertTownSnapshots(ctx, ts, details); err != nil {
		return fmt.Errorf("insert town snapshots: %w", err)
	}

	if err := l.upsertTowns(ctx, ts, details); err != nil {
		return fmt.Errorf("upsert towns: %w", err)
	}

	return nil
}

func (l *LowFreq) insertTownSnapshots(ctx context.Context, ts time.Time, details []json.RawMessage) error {
	if len(details) == 0 {
		return nil
	}

	// Batch insert in chunks to avoid exceeding query param limits
	const chunkSize = 500
	for i := 0; i < len(details); i += chunkSize {
		end := i + chunkSize
		if end > len(details) {
			end = len(details)
		}
		chunk := details[i:end]

		var sb strings.Builder
		sb.WriteString("INSERT INTO town_snapshots (snapshot_ts, town_uuid, town_name, data) VALUES ")

		args := make([]interface{}, 0, len(chunk)*4)
		for j, raw := range chunk {
			name, uuid, err := extractNameUUID(raw)
			if err != nil {
				slog.Warn("skip town: parse error", "error", err)
				continue
			}
			if j > 0 {
				sb.WriteString(",")
			}
			base := j * 4
			sb.WriteString(fmt.Sprintf("($%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4))
			args = append(args, ts, uuid, name, raw)
		}

		if len(args) == 0 {
			continue
		}

		if _, err := l.pool.Exec(ctx, sb.String(), args...); err != nil {
			return fmt.Errorf("batch insert towns %d-%d: %w", i, end, err)
		}
	}
	return nil
}

func (l *LowFreq) upsertTowns(ctx context.Context, ts time.Time, details []json.RawMessage) error {
	if len(details) == 0 {
		return nil
	}

	const chunkSize = 500
	for i := 0; i < len(details); i += chunkSize {
		end := i + chunkSize
		if end > len(details) {
			end = len(details)
		}
		chunk := details[i:end]

		var sb strings.Builder
		sb.WriteString("INSERT INTO towns (uuid, name, first_seen, last_seen) VALUES ")

		args := make([]interface{}, 0, len(chunk)*4)
		count := 0
		for _, raw := range chunk {
			name, uuid, err := extractNameUUID(raw)
			if err != nil {
				continue
			}
			if count > 0 {
				sb.WriteString(",")
			}
			base := count * 4
			sb.WriteString(fmt.Sprintf("($%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4))
			args = append(args, uuid, name, ts, ts)
			count++
		}

		if count == 0 {
			continue
		}

		sb.WriteString(" ON CONFLICT (uuid) DO UPDATE SET name = EXCLUDED.name, last_seen = EXCLUDED.last_seen")

		if _, err := l.pool.Exec(ctx, sb.String(), args...); err != nil {
			return fmt.Errorf("upsert towns %d-%d: %w", i, end, err)
		}
	}
	return nil
}

// ---- Nations ----

func (l *LowFreq) scrapeNations(ctx context.Context, ts time.Time) error {
	nationList, err := l.client.GetNationsList(ctx)
	if err != nil {
		return fmt.Errorf("get nations list: %w", err)
	}
	slog.Info("fetched nation list", "count", len(nationList))

	uuids := make([]string, len(nationList))
	for i, n := range nationList {
		uuids[i] = n.UUID
	}

	details, err := l.client.PostNations(ctx, uuids)
	if err != nil {
		return fmt.Errorf("post nations: %w", err)
	}
	slog.Info("fetched nation details", "count", len(details))

	if err := l.insertNationSnapshots(ctx, ts, details); err != nil {
		return fmt.Errorf("insert nation snapshots: %w", err)
	}

	if err := l.upsertNations(ctx, ts, details); err != nil {
		return fmt.Errorf("upsert nations: %w", err)
	}

	return nil
}

func (l *LowFreq) insertNationSnapshots(ctx context.Context, ts time.Time, details []json.RawMessage) error {
	if len(details) == 0 {
		return nil
	}

	const chunkSize = 500
	for i := 0; i < len(details); i += chunkSize {
		end := i + chunkSize
		if end > len(details) {
			end = len(details)
		}
		chunk := details[i:end]

		var sb strings.Builder
		sb.WriteString("INSERT INTO nation_snapshots (snapshot_ts, nation_uuid, nation_name, data) VALUES ")

		args := make([]interface{}, 0, len(chunk)*4)
		count := 0
		for _, raw := range chunk {
			name, uuid, err := extractNameUUID(raw)
			if err != nil {
				slog.Warn("skip nation: parse error", "error", err)
				continue
			}
			if count > 0 {
				sb.WriteString(",")
			}
			base := count * 4
			sb.WriteString(fmt.Sprintf("($%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4))
			args = append(args, ts, uuid, name, raw)
			count++
		}

		if count == 0 {
			continue
		}

		if _, err := l.pool.Exec(ctx, sb.String(), args...); err != nil {
			return fmt.Errorf("batch insert nations %d-%d: %w", i, end, err)
		}
	}
	return nil
}

func (l *LowFreq) upsertNations(ctx context.Context, ts time.Time, details []json.RawMessage) error {
	if len(details) == 0 {
		return nil
	}

	const chunkSize = 500
	for i := 0; i < len(details); i += chunkSize {
		end := i + chunkSize
		if end > len(details) {
			end = len(details)
		}
		chunk := details[i:end]

		var sb strings.Builder
		sb.WriteString("INSERT INTO nations (uuid, name, first_seen, last_seen) VALUES ")

		args := make([]interface{}, 0, len(chunk)*4)
		count := 0
		for _, raw := range chunk {
			name, uuid, err := extractNameUUID(raw)
			if err != nil {
				continue
			}
			if count > 0 {
				sb.WriteString(",")
			}
			base := count * 4
			sb.WriteString(fmt.Sprintf("($%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4))
			args = append(args, uuid, name, ts, ts)
			count++
		}

		if count == 0 {
			continue
		}

		sb.WriteString(" ON CONFLICT (uuid) DO UPDATE SET name = EXCLUDED.name, last_seen = EXCLUDED.last_seen")

		if _, err := l.pool.Exec(ctx, sb.String(), args...); err != nil {
			return fmt.Errorf("upsert nations %d-%d: %w", i, end, err)
		}
	}
	return nil
}

// ---- Players (full profile) ----

func (l *LowFreq) scrapePlayers(ctx context.Context, ts time.Time) error {
	playerList, err := l.client.GetPlayersList(ctx)
	if err != nil {
		return fmt.Errorf("get players list: %w", err)
	}
	slog.Info("fetched player list", "count", len(playerList))

	uuids := make([]string, len(playerList))
	for i, p := range playerList {
		uuids[i] = p.UUID
	}

	details, err := l.client.PostPlayers(ctx, uuids)
	if err != nil {
		return fmt.Errorf("post players: %w", err)
	}
	slog.Info("fetched player details", "count", len(details))

	if err := l.insertPlayerSnapshots(ctx, ts, details); err != nil {
		return fmt.Errorf("insert player snapshots: %w", err)
	}

	// Also upsert the players dimension table
	if err := l.upsertPlayersFull(ctx, ts, details); err != nil {
		return fmt.Errorf("upsert players: %w", err)
	}

	return nil
}

func (l *LowFreq) insertPlayerSnapshots(ctx context.Context, ts time.Time, details []json.RawMessage) error {
	if len(details) == 0 {
		return nil
	}

	const chunkSize = 500
	for i := 0; i < len(details); i += chunkSize {
		end := i + chunkSize
		if end > len(details) {
			end = len(details)
		}
		chunk := details[i:end]

		var sb strings.Builder
		sb.WriteString("INSERT INTO player_snapshots (snapshot_ts, player_uuid, player_name, data) VALUES ")

		args := make([]interface{}, 0, len(chunk)*4)
		count := 0
		for _, raw := range chunk {
			name, uuid, err := extractNameUUID(raw)
			if err != nil {
				slog.Warn("skip player: parse error", "error", err)
				continue
			}
			if count > 0 {
				sb.WriteString(",")
			}
			base := count * 4
			sb.WriteString(fmt.Sprintf("($%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4))
			args = append(args, ts, uuid, name, raw)
			count++
		}

		if count == 0 {
			continue
		}

		if _, err := l.pool.Exec(ctx, sb.String(), args...); err != nil {
			return fmt.Errorf("batch insert players %d-%d: %w", i, end, err)
		}
	}
	return nil
}

func (l *LowFreq) upsertPlayersFull(ctx context.Context, ts time.Time, details []json.RawMessage) error {
	if len(details) == 0 {
		return nil
	}

	const chunkSize = 500
	for i := 0; i < len(details); i += chunkSize {
		end := i + chunkSize
		if end > len(details) {
			end = len(details)
		}
		chunk := details[i:end]

		var sb strings.Builder
		sb.WriteString("INSERT INTO players (uuid, name, first_seen, last_seen) VALUES ")

		args := make([]interface{}, 0, len(chunk)*4)
		count := 0
		for _, raw := range chunk {
			name, uuid, err := extractNameUUID(raw)
			if err != nil {
				continue
			}
			if count > 0 {
				sb.WriteString(",")
			}
			base := count * 4
			sb.WriteString(fmt.Sprintf("($%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4))
			args = append(args, uuid, name, ts, ts)
			count++
		}

		if count == 0 {
			continue
		}

		sb.WriteString(" ON CONFLICT (uuid) DO UPDATE SET name = EXCLUDED.name, last_seen = EXCLUDED.last_seen")

		if _, err := l.pool.Exec(ctx, sb.String(), args...); err != nil {
			return fmt.Errorf("upsert players %d-%d: %w", i, end, err)
		}
	}
	return nil
}
