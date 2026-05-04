-- Per-server BattlEye anti-cheat toggle. Defaults to 1 (enabled),
-- which matches what ARK ships and what existing servers expect on
-- their next launch — no behavior change without explicit user action.
-- The launch builder appends "-NoBattlEye" only when this is 0.

ALTER TABLE servers ADD COLUMN anticheat_enabled INTEGER NOT NULL DEFAULT 1;
