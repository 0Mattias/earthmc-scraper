# EarthMC Tracker — Backend & Database

This repository contains the Go-based Cloud Run worker that powers the **EarthMC Tracker**. It continuously scrapes the EarthMC API and live map, building a rich, historically-accurate PostgreSQL database designed for time-series analysis and frontend data visualization.

---

## 🏗️ Database Architecture

The database is built for extreme write-throughput and flexible reads. It is split into three main categories of tables.

### 1. High-Frequency Tracking: `player_activity`
* **Scrape Interval**: Every 3 seconds
* **Purpose**: Tracking exact player movements, online status, and map visibility.
* **Structure**: Partitioned by the hour (e.g., `player_activity_20260228_150000`). This ensures lightning-fast queries for specific timeframes without scanning millions of rows.
* **Key Columns**: `snapshot_ts` (timestamp), `player_uuid`, `player_name`, `is_online`, `is_visible`, `x`, `y`, `z`, `world`

### 2. Low-Frequency Snapshots: `*_snapshots`
* **Scrape Interval**: Every 3 minutes
* **Purpose**: Storing the complete state of the server, towns, nations, and players over time.
* **Structure**: We use PostgreSQL's `JSONB` data type to store the *exact* API response. This allows the EarthMC API to add or change fields without breaking our database schema.
* **Tables**: 
  - `server_snapshots` (basic columns, no JSONB needed)
  - `player_snapshots` (JSONB)
  - `town_snapshots` (JSONB)
  - `nation_snapshots` (JSONB)

### 3. Dimension Tables: `players`, `towns`, `nations`
* **Purpose**: Fast lookups for entities we've seen, tracking when they were first and last seen by the scraper.
* **Structure**: Upserted constantly. Simple UUID, Name, First Seen, Last Seen columns.

---

## 💻 Querying for the Frontend

Here are common SQL patterns you will use when building the EarthMC Tracker UI.

### 🗺️ Use Case 1: Draw a Player's Path on the Map
To draw the breadcrumb trail of where a player has been over the last hour:

```sql
SELECT snapshot_ts, x, y, z, world
FROM player_activity
WHERE player_name = 'TargetPlayerName'
  AND is_visible = true
  AND snapshot_ts >= NOW() - INTERVAL '1 hour'
ORDER BY snapshot_ts ASC;
```

### 📈 Use Case 2: Town Population/Balance History
Because the details are stored in `JSONB`, we can extract historical stats easily:

```sql
SELECT 
    snapshot_ts,
    (data->'stats'->>'numResidents')::int AS resident_count,
    (data->'stats'->>'balance')::numeric AS town_bank
FROM town_snapshots
WHERE town_name = 'TargetTown'
ORDER BY snapshot_ts DESC
LIMIT 50; -- The last 50 snapshots (~2.5 hours of history)
```

### 👑 Use Case 3: Nation Details Extract
Extracting the King, Capital, and number of towns perfectly at a specific point in time:

```sql
SELECT 
    nation_name, 
    snapshot_ts,
    data->'king'->>'name' AS king,
    data->'capital'->>'name' AS capital,
    (data->'stats'->>'numTowns')::int AS current_town_count
FROM nation_snapshots
WHERE nation_name = 'TargetNation'
ORDER BY snapshot_ts DESC
LIMIT 1; -- Gets the absolute freshest data
```

### ⏱️ Use Case 4: Who was online at a specific time in the past?
Thanks to the partitioned timestamp indexing, looking up history is instant:

```sql
-- "Who was online yesterday at exactly noon?"
SELECT player_name, world, x, z
FROM player_activity
WHERE snapshot_ts BETWEEN '2026-02-28 12:00:00+00' AND '2026-02-28 12:00:05+00'
  AND is_online = true;
```

---

## ⚙️ How partitions work
If you look at the database in Cloud SQL Studio, you will see tables like `player_activity_20260301_110000`. **Do not query these directly.** 

These are just physical storage "buckets" the database uses internally. The scraper creates them 48 hours in advance automatically. **Always query the main `player_activity` table**, and Postgres will automatically fetch the data from the correct bucket behind the scenes.
