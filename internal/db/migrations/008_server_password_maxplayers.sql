-- Per-server player join password and player slot cap.
-- server_password defaults to '' (no password, server is open).
-- max_players defaults to 0, which means "do not pass -WinLiveMaxPlayers"
-- and let ASA use its built-in default (70).

ALTER TABLE servers ADD COLUMN server_password TEXT NOT NULL DEFAULT '';
ALTER TABLE servers ADD COLUMN max_players INTEGER NOT NULL DEFAULT 0;
