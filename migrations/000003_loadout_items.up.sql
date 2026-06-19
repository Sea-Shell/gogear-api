-- Create loadout_items table for gear within loadouts

CREATE TABLE IF NOT EXISTS loadout_items (
    loadoutItemId INTEGER PRIMARY KEY AUTOINCREMENT,
    loadoutId INTEGER NOT NULL,
    gearId INTEGER NOT NULL,
    quantity INTEGER NOT NULL DEFAULT 1,
    notes TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (loadoutId) REFERENCES loadouts(loadoutId) ON DELETE CASCADE,
    FOREIGN KEY (gearId) REFERENCES gear(gearId)
);

CREATE INDEX IF NOT EXISTS idx_loadout_items_loadout ON loadout_items(loadoutId);
CREATE INDEX IF NOT EXISTS idx_loadout_items_gear ON loadout_items(gearId);
