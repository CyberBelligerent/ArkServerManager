-- Add free-form custom INI line support to clusters. Each entry in
-- the JSON array is {File, Section, Key, Value}; the cluster's
-- WriteServerINIs appends them after the registered settings. Used
-- for mod-specific config keys we don't model in the registry.

ALTER TABLE clusters ADD COLUMN custom_lines_json TEXT NOT NULL DEFAULT '[]';
