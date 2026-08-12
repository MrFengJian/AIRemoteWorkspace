-- AI Remote Workspace — SQLite schema
--
-- Design intent (AGENT.md §8, §9):
--   * Plain SQLite (no SQLCipher) — keep CGO off the table for a single-binary
--     desktop app. Field-level secrets never live here; only `secret_ref`.
--   * Schema is versioned via `schema_version`. Future migrations bump it and
--     run idempotently on startup.

-- Tracks applied schema version for forward-only migrations.
CREATE TABLE IF NOT EXISTS schema_version (
    version    INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Hosts (Phase 2 fills CRUD + connection state).
-- `secret_ref` is the only persisted credential field: an opaque handle into
-- the OS keychain (AGENT.md §9). Never store password / key bytes here.
CREATE TABLE IF NOT EXISTS hosts (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    host       TEXT NOT NULL,
    port       INTEGER NOT NULL DEFAULT 22,
    username   TEXT NOT NULL,
    auth_type  TEXT NOT NULL DEFAULT 'password',
    secret_ref TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Sessions history (connection lifecycle). Populated from Phase 2 onward.
CREATE TABLE IF NOT EXISTS sessions (
    id         TEXT PRIMARY KEY,
    host_id    TEXT NOT NULL,
    started_at TEXT NOT NULL DEFAULT (datetime('now')),
    ended_at   TEXT,
    status     TEXT NOT NULL DEFAULT 'connecting',
    FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE
);

-- Application settings. Single row keyed by 'app'.
CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Known SSH host keys (known_hosts style, AGENT.md §8 security).
-- First connection records the fingerprint; subsequent connections verify.
CREATE TABLE IF NOT EXISTS host_keys (
    host_id     TEXT NOT NULL,
    algorithm   TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    first_seen  TEXT NOT NULL DEFAULT (datetime('now')),
    last_seen   TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (host_id, algorithm),
    FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE
);

-- Secret references placeholder (Phase 5 SecretStore). Created now so the
-- schema is stable; rows are only written once the OS keychain layer lands.
CREATE TABLE IF NOT EXISTS secret_refs (
    ref      TEXT PRIMARY KEY,
    kind     TEXT NOT NULL,            -- 'password' | 'key' | 'token'
    host_id  TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
