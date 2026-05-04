-- Cluster-level mod list. Servers' own mods append to this, deduped
-- and order-preserved (cluster first, server next), so an admin can
-- pin a baseline mod set across every server in the cluster while
-- letting individual maps add their own.
--
-- Default '[]' matches servers.active_mods_json so existing clusters
-- behave as if they had no cluster-level mods (current behavior).

ALTER TABLE clusters ADD COLUMN active_mods_json TEXT NOT NULL DEFAULT '[]';
