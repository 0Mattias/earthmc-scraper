-- EarthMC Scraper: Initial Schema
-- Designed for time-series snapshot analysis with partitioning

-- ============================================================
-- Dimension Tables (slowly-changing, upserted)
-- ============================================================

CREATE TABLE IF NOT EXISTS players (
    uuid        TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    first_seen  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_players_name ON players(name);

CREATE TABLE IF NOT EXISTS towns (
    uuid        TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    first_seen  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_towns_name ON towns(name);

CREATE TABLE IF NOT EXISTS nations (
    uuid        TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    first_seen  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_nations_name ON nations(name);

-- ============================================================
-- High-frequency: Player Activity (every 3s)
-- Partitioned by month on snapshot_ts
-- ============================================================

CREATE TABLE IF NOT EXISTS player_activity (
    id           BIGSERIAL,
    snapshot_ts  TIMESTAMPTZ NOT NULL,
    player_uuid  TEXT NOT NULL,
    player_name  TEXT NOT NULL,
    is_online    BOOLEAN NOT NULL DEFAULT TRUE,
    is_visible   BOOLEAN NOT NULL DEFAULT FALSE,
    x            INTEGER,
    y            INTEGER,
    z            INTEGER,
    yaw          INTEGER,
    world        TEXT,
    PRIMARY KEY (id, snapshot_ts)
) PARTITION BY RANGE (snapshot_ts);

-- Create partitions for the next 12 months
DO $$
DECLARE
    start_date DATE := DATE_TRUNC('month', CURRENT_DATE);
    end_date DATE;
    partition_name TEXT;
BEGIN
    FOR i IN 0..11 LOOP
        end_date := start_date + INTERVAL '1 month';
        partition_name := 'player_activity_' || TO_CHAR(start_date, 'YYYY_MM');
        
        IF NOT EXISTS (SELECT 1 FROM pg_class WHERE relname = partition_name) THEN
            EXECUTE FORMAT(
                'CREATE TABLE %I PARTITION OF player_activity FOR VALUES FROM (%L) TO (%L)',
                partition_name, start_date, end_date
            );
        END IF;
        
        start_date := end_date;
    END LOOP;
END $$;

-- BRIN index for fast range scans on the partitioned table
CREATE INDEX IF NOT EXISTS idx_player_activity_ts_brin ON player_activity USING BRIN (snapshot_ts);
CREATE INDEX IF NOT EXISTS idx_player_activity_player ON player_activity (player_uuid, snapshot_ts);

-- ============================================================
-- Low-frequency: Server Snapshots (every 3 min)
-- ============================================================

CREATE TABLE IF NOT EXISTS server_snapshots (
    id                   BIGSERIAL PRIMARY KEY,
    snapshot_ts          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    version              TEXT,
    moon_phase           TEXT,
    has_storm            BOOLEAN,
    is_thundering        BOOLEAN,
    server_time          BIGINT,
    full_time            BIGINT,
    max_players          INTEGER,
    num_online_players   INTEGER,
    num_online_nomads    INTEGER,
    num_residents        INTEGER,
    num_nomads           INTEGER,
    num_towns            INTEGER,
    num_town_blocks      INTEGER,
    num_nations          INTEGER,
    num_quarters         INTEGER,
    num_cuboids          INTEGER,
    vote_party_target    INTEGER,
    vote_party_remaining INTEGER
);
CREATE INDEX IF NOT EXISTS idx_server_snapshots_ts ON server_snapshots (snapshot_ts);

-- ============================================================
-- Low-frequency: Player Snapshots (every 3 min)
-- Full player profile stored as JSONB for flexibility
-- ============================================================

CREATE TABLE IF NOT EXISTS player_snapshots (
    id           BIGSERIAL PRIMARY KEY,
    snapshot_ts  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    player_uuid  TEXT NOT NULL,
    player_name  TEXT NOT NULL,
    data         JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_player_snapshots_ts ON player_snapshots (snapshot_ts);
CREATE INDEX IF NOT EXISTS idx_player_snapshots_player ON player_snapshots (player_uuid, snapshot_ts);
CREATE INDEX IF NOT EXISTS idx_player_snapshots_data ON player_snapshots USING GIN (data);

-- ============================================================
-- Low-frequency: Town Snapshots (every 3 min)
-- ============================================================

CREATE TABLE IF NOT EXISTS town_snapshots (
    id          BIGSERIAL PRIMARY KEY,
    snapshot_ts TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    town_uuid   TEXT NOT NULL,
    town_name   TEXT NOT NULL,
    data        JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_town_snapshots_ts ON town_snapshots (snapshot_ts);
CREATE INDEX IF NOT EXISTS idx_town_snapshots_town ON town_snapshots (town_uuid, snapshot_ts);
CREATE INDEX IF NOT EXISTS idx_town_snapshots_data ON town_snapshots USING GIN (data);

-- ============================================================
-- Low-frequency: Nation Snapshots (every 3 min)
-- ============================================================

CREATE TABLE IF NOT EXISTS nation_snapshots (
    id          BIGSERIAL PRIMARY KEY,
    snapshot_ts TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    nation_uuid TEXT NOT NULL,
    nation_name TEXT NOT NULL,
    data        JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_nation_snapshots_ts ON nation_snapshots (snapshot_ts);
CREATE INDEX IF NOT EXISTS idx_nation_snapshots_nation ON nation_snapshots (nation_uuid, snapshot_ts);
CREATE INDEX IF NOT EXISTS idx_nation_snapshots_data ON nation_snapshots USING GIN (data);
