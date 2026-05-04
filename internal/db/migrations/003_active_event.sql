-- Add ActiveEvent column to clusters and servers. Empty string means no
-- event; the resolver treats server-empty as "inherit from cluster" and
-- the literal string "None" as "explicitly cleared". The launch builder
-- only emits the -ActiveEvent= flag for non-empty, non-None values.

ALTER TABLE clusters ADD COLUMN active_event TEXT NOT NULL DEFAULT '';
ALTER TABLE servers  ADD COLUMN active_event TEXT NOT NULL DEFAULT '';
