-- Settings presets are diff-style overrides scoped to a cluster or a
-- server. payload_json is the entire diff (settings to set, optional
-- active_event override). Apply semantics: only keys present in the
-- payload are written; everything else is left alone.
--
-- UNIQUE(scope_type, scope_id, name) prevents duplicate names within a
-- single scope so dropdowns stay disambiguated, but the same name can
-- exist across scopes (e.g. "Summer" on multiple clusters).

CREATE TABLE settings_presets (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    scope_type    TEXT NOT NULL CHECK(scope_type IN ('cluster','server')),
    scope_id      INTEGER NOT NULL,
    name          TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    payload_json  TEXT NOT NULL DEFAULT '{}',
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(scope_type, scope_id, name)
);

CREATE INDEX idx_presets_scope ON settings_presets(scope_type, scope_id);
