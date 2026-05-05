-- Per-server toggle for RCON. Default 1 (enabled) so existing servers
-- behave exactly as before. When set to 0 the launch builder omits the
-- RCONEnabled / ServerAdminPassword chain entries, the supervisor skips
-- its RCON readiness probe, and player tracking + graceful stop become
-- no-ops for that server.

ALTER TABLE servers ADD COLUMN rcon_enabled INTEGER NOT NULL DEFAULT 1;
