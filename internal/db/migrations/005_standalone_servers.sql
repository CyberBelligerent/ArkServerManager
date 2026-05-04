-- Allow servers to exist outside any cluster (cluster_id NULL).

CREATE TABLE servers_new (
    id                       INTEGER PRIMARY KEY AUTOINCREMENT,
    cluster_id               INTEGER REFERENCES clusters(id) ON DELETE CASCADE,
    name                     TEXT NOT NULL,
    map                      TEXT NOT NULL,
    install_dir              TEXT NOT NULL,
    port                     INTEGER NOT NULL,
    query_port               INTEGER NOT NULL,
    rcon_port                INTEGER NOT NULL,
    rcon_password            TEXT NOT NULL DEFAULT '',
    settings_overrides_json  TEXT NOT NULL DEFAULT '{}',
    active_mods_json         TEXT NOT NULL DEFAULT '[]',
    active_event             TEXT NOT NULL DEFAULT '',
    status                   TEXT NOT NULL DEFAULT 'stopped',
    created_at               TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO servers_new (
    id, cluster_id, name, map, install_dir,
    port, query_port, rcon_port, rcon_password,
    settings_overrides_json, active_mods_json, active_event,
    status, created_at
) SELECT
    id, cluster_id, name, map, install_dir,
    port, query_port, rcon_port, rcon_password,
    settings_overrides_json, active_mods_json, active_event,
    status, created_at
FROM servers;

DROP TABLE servers;
ALTER TABLE servers_new RENAME TO servers;

CREATE INDEX idx_servers_cluster_id ON servers(cluster_id);
CREATE UNIQUE INDEX idx_servers_cluster_name ON servers(cluster_id, name);
