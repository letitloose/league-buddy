-- Registered location names had drifted from the shorter names used in the
-- Fall 2026 schedule CSV; align them. Matched by name, not id, same reason
-- as migration 0013's team renames (dev's RESETDB=true reshuffles ids on
-- every restart; prod ids are never guaranteed to match dev's).
UPDATE locations SET name = 'East Greenbush' WHERE name = 'East Greenbush Soccer Club';
UPDATE locations SET name = 'Jenkinsville Rd' WHERE name = 'Jenkinsville Rd Fields';
UPDATE locations SET name = 'Nott Road' WHERE name = 'Nott Road Fields';

-- New location referenced by the Fall 2026 schedule but never registered.
-- Guarded by addressKey (the table's actual uniqueness constraint, same
-- one LocationService.CreateLocation dedupes on) so this stays safe if
-- ever re-run against a DB where it was already added another way.
INSERT INTO address (address1, city, stateProvince)
SELECT 'Mohonasen High School Schenectady', 'Schenectady', 'NY'
WHERE NOT EXISTS (
    SELECT 1 FROM locations WHERE addressKey = 'mohonasen high school schenectady||schenectady|ny|'
);
INSERT INTO locations (name, addressID, addressKey, created)
SELECT 'Mohonasen High School Schenectady', LAST_INSERT_ID(), 'mohonasen high school schenectady||schenectady|ny|', NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM locations WHERE addressKey = 'mohonasen high school schenectady||schenectady|ny|'
);
