-- Remote Agent PostgreSQL Schema
-- Run by docker-entrypoint-initdb.d on first postgres start

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Users (for dashboard authentication)
CREATE TABLE IF NOT EXISTS users (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email       TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    totp_secret TEXT,
    is_admin    BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Registered devices/agents
CREATE TABLE IF NOT EXISTS devices (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id    TEXT NOT NULL UNIQUE,  -- 9-digit display ID e.g. "123-456-789"
    name        TEXT NOT NULL,
    hostname    TEXT,
    os          TEXT,
    version     TEXT,
    owner_id    UUID REFERENCES users(id) ON DELETE SET NULL,
    last_seen   TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_devices_agent_id ON devices(agent_id);
CREATE INDEX IF NOT EXISTS idx_devices_owner_id ON devices(owner_id);

-- Session audit log
CREATE TABLE IF NOT EXISTS sessions (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    host_agent_id   TEXT NOT NULL,
    controller_ip   TEXT,
    controller_user UUID REFERENCES users(id) ON DELETE SET NULL,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at        TIMESTAMPTZ,
    duration_secs   INTEGER GENERATED ALWAYS AS (
        EXTRACT(EPOCH FROM (ended_at - started_at))::INTEGER
    ) STORED,
    end_reason      TEXT,  -- 'normal', 'timeout', 'error', 'forced'
    bytes_relayed   BIGINT DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_sessions_host    ON sessions(host_agent_id);
CREATE INDEX IF NOT EXISTS idx_sessions_started ON sessions(started_at DESC);

-- Access control: which controllers can access which hosts
CREATE TABLE IF NOT EXISTS access_rules (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    host_agent_id   TEXT NOT NULL,
    controller_id   UUID REFERENCES users(id) ON DELETE CASCADE,
    permission  TEXT NOT NULL DEFAULT 'control',  -- 'view' | 'control'
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (host_agent_id, controller_id)
);
