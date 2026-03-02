# EarthMC Scraper — Backend Worker & Database

This repository contains the Go-based Cloud Run worker that powers the **EarthMC Tracker**. It continuously scrapes the EarthMC API and live map, building a rich, historically-accurate PostgreSQL database designed for time-series analysis and frontend data visualization.

This application is designed specifically so that an AI agent built into the site can easily query these records and make insights using the Vertex AI integration. 

---

## 🏗️ Scraper Architecture

The scraper operates using two distinct loops to efficiently gather data from the EarthMC endpoints without violating rate limits or taking unnecessarily heavy snapshots.

### 1. The High-Frequency Loop (Every 3 seconds)
This loop is responsible for precise tracking of player activity, coordinates, and online status. 
- Queries `https://map.earthmc.net/tiles/players.json` to see who is on the live map along with their XYZ coordinates.
- Queries `https://api.earthmc.net/v3/aurora/online` to get the definitive list of online players.
- **Deduction Logic:** Reconciles the two endpoints. Everyone on the live map is marked as `is_visible=true`. Everyone in the `/online` endpoint is marked as `is_online=true`.
- **Database Target:** Quickly batch-inserts records into the `player_activity` partitioned table. 

### 2. The Low-Frequency Loop (Every 3 minutes)
This loop captures the heavy, detailed state of the server, players, towns, and nations.
- Queries the root Server stats, `.../towns`, `.../nations`, and `.../players` lists.
- Uses `POST` batch endpoints to fetch full data objects for all entities.
- **Database Target:** Stores the raw JSON responses directly into PostgreSQL `JSONB` columns in the `*_snapshots` tables. Upserts the `players`, `towns`, and `nations` dimension tables.

---

## 🗄️ Database Schema & Partitioning

The database is built for extreme write-throughput (via the 3-second loop) and flexible reads.

### ⏱️ Partitioning & pg_cron
The `player_activity` table generates a massive amount of rows. To ensure queries remain lightning-fast, it uses PostgreSQL Range Partitioning.
- The scraper (and a background `pg_cron` job in the DB) automatically pre-creates hourly partitions **30 days (720 hours) in advance**.
- Example partition: `player_activity_20260228_150000`
- **Important for AI Agents:** Do not query these partition buckets directly. Always query the parent `player_activity` table and use the `snapshot_ts` timestamp column to filter by time. Postgres will efficiently route the query to the correct buckets. 

### Fully Defined Schema
Below is the exact schema implemented in the database. AI agents can use this to construct perfect SQL queries.

```sql
-- Dimension Tables (slowly-changing, upserted)
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

-- High-frequency: Player Activity (every 3s)
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

CREATE INDEX IF NOT EXISTS idx_player_activity_ts_brin ON player_activity USING BRIN (snapshot_ts);
CREATE INDEX IF NOT EXISTS idx_player_activity_player ON player_activity (player_uuid, snapshot_ts);

-- Low-frequency: Server Snapshots (every 3 min)
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

-- Low-frequency: Player Snapshots (every 3 min)
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

-- Low-frequency: Town Snapshots (every 3 min)
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

-- Low-frequency: Nation Snapshots (every 3 min)
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
```

---

## 💻 Example Queries for AI Agents

Here are common SQL patterns an AI Agent could use to retrieve intelligence:

### 🗺️ Target a Player's Movements
To get the breadcrumb trail of where a player has been over the last hour:
```sql
SELECT snapshot_ts, x, y, z, world
FROM player_activity
WHERE player_name = 'TargetPlayerName'
  AND is_visible = true
  AND snapshot_ts >= NOW() - INTERVAL '1 hour'
ORDER BY snapshot_ts ASC;
```

### 📈 Town Population History
Extracting historical stats perfectly out of the `JSONB` data:
```sql
SELECT 
    snapshot_ts,
    (data->'stats'->>'numResidents')::int AS resident_count,
    (data->'stats'->>'balance')::numeric AS town_bank
FROM town_snapshots
WHERE town_name = 'TargetTown'
ORDER BY snapshot_ts DESC
LIMIT 50; 
```

### ⏱️ Point-In-Time Online Status
Leveraging partition indexing for instant historical lookups:
```sql
SELECT player_name, world, x, z
FROM player_activity
WHERE snapshot_ts BETWEEN '2026-02-28 12:00:00+00' AND '2026-02-28 12:00:05+00'
  AND is_online = true;
```
