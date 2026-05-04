-- Core entities

CREATE TABLE clusters (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL,
    cluster_id      TEXT NOT NULL UNIQUE,
    cluster_dir     TEXT NOT NULL,
    settings_json   TEXT NOT NULL DEFAULT '{}',
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE servers (
    id                       INTEGER PRIMARY KEY AUTOINCREMENT,
    cluster_id               INTEGER NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    name                     TEXT NOT NULL,
    map                      TEXT NOT NULL,
    install_dir              TEXT NOT NULL,
    port                     INTEGER NOT NULL,
    query_port               INTEGER NOT NULL,
    rcon_port                INTEGER NOT NULL,
    rcon_password            TEXT NOT NULL DEFAULT '',
    settings_overrides_json  TEXT NOT NULL DEFAULT '{}',
    active_mods_json         TEXT NOT NULL DEFAULT '[]',
    status                   TEXT NOT NULL DEFAULT 'stopped',
    created_at               TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_servers_cluster_id ON servers(cluster_id);
CREATE UNIQUE INDEX idx_servers_cluster_name ON servers(cluster_id, name);

-- Mod registry per server + cache for the CurseForge browser

CREATE TABLE mods (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id       INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    curseforge_id   INTEGER NOT NULL,
    name            TEXT NOT NULL,
    version         TEXT,
    enabled         INTEGER NOT NULL DEFAULT 1
);

CREATE UNIQUE INDEX idx_mods_server_curseforge ON mods(server_id, curseforge_id);

CREATE TABLE mod_cache (
    curseforge_id   INTEGER PRIMARY KEY,
    payload_json    TEXT NOT NULL,
    fetched_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Backups + templates

CREATE TABLE backups (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    scope_type  TEXT NOT NULL CHECK (scope_type IN ('server','cluster')),
    scope_id    INTEGER NOT NULL,
    path        TEXT NOT NULL,
    size_bytes  INTEGER NOT NULL DEFAULT 0,
    kind        TEXT NOT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_backups_scope ON backups(scope_type, scope_id, created_at DESC);

CREATE TABLE templates (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL UNIQUE,
    payload_json TEXT NOT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Player management

CREATE TABLE players (
    steam_id        TEXT PRIMARY KEY,
    last_known_name TEXT NOT NULL,
    first_seen      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    total_minutes   INTEGER NOT NULL DEFAULT 0,
    notes           TEXT NOT NULL DEFAULT '',
    is_watched      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE player_name_history (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    steam_id    TEXT NOT NULL REFERENCES players(steam_id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    observed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_pnh_steam_id ON player_name_history(steam_id);

CREATE TABLE player_sessions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    steam_id    TEXT NOT NULL REFERENCES players(steam_id) ON DELETE CASCADE,
    server_id   INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    joined_at   TIMESTAMP NOT NULL,
    left_at     TIMESTAMP
);

CREATE INDEX idx_sessions_player ON player_sessions(steam_id);
CREATE INDEX idx_sessions_server ON player_sessions(server_id);

CREATE TABLE bans (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    steam_id    TEXT NOT NULL,
    scope_type  TEXT NOT NULL CHECK (scope_type IN ('server','cluster')),
    scope_id    INTEGER NOT NULL,
    reason      TEXT NOT NULL DEFAULT '',
    banned_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    banned_by   TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_bans_steam ON bans(steam_id);
CREATE UNIQUE INDEX idx_bans_unique ON bans(steam_id, scope_type, scope_id);

CREATE TABLE whitelist (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    steam_id    TEXT NOT NULL,
    scope_type  TEXT NOT NULL CHECK (scope_type IN ('server','cluster')),
    scope_id    INTEGER NOT NULL,
    added_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX idx_whitelist_unique ON whitelist(steam_id, scope_type, scope_id);

-- Discord

CREATE TABLE discord_webhooks (
    id                       INTEGER PRIMARY KEY AUTOINCREMENT,
    name                     TEXT NOT NULL,
    url                      TEXT NOT NULL,
    scope_type               TEXT NOT NULL CHECK (scope_type IN ('global','cluster','server')),
    scope_id                 INTEGER,
    event_mask               TEXT NOT NULL DEFAULT '[]',
    template_overrides_json  TEXT NOT NULL DEFAULT '{}',
    enabled                  INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE discord_bot_config (
    id                INTEGER PRIMARY KEY CHECK (id = 1),
    token_encrypted   BLOB,
    guild_id          TEXT,
    enabled           INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE discord_role_perms (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    guild_id    TEXT NOT NULL,
    role_id     TEXT NOT NULL,
    command     TEXT NOT NULL,
    allowed     INTEGER NOT NULL DEFAULT 1
);

CREATE UNIQUE INDEX idx_role_perms_unique ON discord_role_perms(guild_id, role_id, command);

CREATE TABLE discord_command_audit (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id      TEXT NOT NULL,
    user_name    TEXT NOT NULL,
    command      TEXT NOT NULL,
    args_json    TEXT NOT NULL DEFAULT '{}',
    result       TEXT NOT NULL,
    executed_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Scheduler

CREATE TABLE scheduled_tasks (
    id                       INTEGER PRIMARY KEY AUTOINCREMENT,
    name                     TEXT NOT NULL,
    scope_type               TEXT NOT NULL CHECK (scope_type IN ('global','cluster','server')),
    scope_id                 INTEGER,
    trigger_kind             TEXT NOT NULL CHECK (trigger_kind IN ('oneshot','cron','window','recurring_window')),
    trigger_payload_json     TEXT NOT NULL DEFAULT '{}',
    action_kind              TEXT NOT NULL,
    action_payload_json      TEXT NOT NULL DEFAULT '{}',
    start_at                 TIMESTAMP,
    end_at                   TIMESTAMP,
    cron_expr                TEXT,
    recurrence               TEXT NOT NULL DEFAULT '',
    missed_policy            TEXT NOT NULL DEFAULT 'skip' CHECK (missed_policy IN ('skip','run_once','apply_state')),
    status                   TEXT NOT NULL DEFAULT 'enabled' CHECK (status IN ('enabled','disabled')),
    last_fired_at            TIMESTAMP,
    next_fire_at             TIMESTAMP,
    created_at               TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_tasks_next_fire ON scheduled_tasks(next_fire_at) WHERE status = 'enabled';

CREATE TABLE scheduled_task_runs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id     INTEGER NOT NULL REFERENCES scheduled_tasks(id) ON DELETE CASCADE,
    fired_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    result      TEXT NOT NULL,
    message     TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_task_runs_task ON scheduled_task_runs(task_id, fired_at DESC);

CREATE TABLE active_windows (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id         INTEGER NOT NULL REFERENCES scheduled_tasks(id) ON DELETE CASCADE,
    scope_type      TEXT NOT NULL CHECK (scope_type IN ('cluster','server')),
    scope_id        INTEGER NOT NULL,
    applied_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reverts_to_json TEXT NOT NULL DEFAULT '{}'
);

CREATE UNIQUE INDEX idx_active_windows_task ON active_windows(task_id, scope_type, scope_id);
