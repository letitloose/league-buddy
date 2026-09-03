-- Registered team names had drifted from what the league now calls them;
-- align to the names used in the Fall 2026 schedule CSV before importing it.
-- Matched by name, not id: dev's RESETDB=true reshuffles auto-increment ids
-- on every restart, and prod ids are never guaranteed to line up with dev's.
UPDATE teams SET name = 'Chatham' WHERE name = 'Chatham SOMA';
UPDATE teams SET name = 'Atletico FC' WHERE name = 'Atletico Clifton Park';
UPDATE teams SET name = 'Guilderland FC' WHERE name = 'Guilderland FC - Marisa''s Lace Pizzeria Rest.';
UPDATE teams SET name = 'Greene FC' WHERE name = 'Greene FC - I.T.S. Inc';
-- Per the site owner, these are the same teams under earlier names, not
-- renames onto a different team's identity.
UPDATE teams SET name = 'Fulmont FC' WHERE name = 'GSL FC - Active Ingredient Brewing Co';
UPDATE teams SET name = 'Grassmonkey FC' WHERE name = 'Crazy Dawg T Shirts';

-- New teams that appear in the Fall 2026 schedule but were never registered.
-- Derives the league from Colonial FC rather than a hardcoded league id,
-- since that id isn't guaranteed to match between dev and prod. Guarded
-- with NOT EXISTS so this stays safe if ever re-run against a DB where
-- these were already added another way (e.g. dev's seed data).
INSERT INTO teams (leagueID, name, created)
SELECT c.leagueID, 'Resurgence FC', NOW() FROM teams c WHERE c.name = 'Colonial FC'
AND NOT EXISTS (SELECT 1 FROM teams WHERE name = 'Resurgence FC');
INSERT INTO teams (leagueID, name, created)
SELECT c.leagueID, 'Rotterdam', NOW() FROM teams c WHERE c.name = 'Colonial FC'
AND NOT EXISTS (SELECT 1 FROM teams WHERE name = 'Rotterdam');
INSERT INTO teams (leagueID, name, created)
SELECT c.leagueID, 'Troy United FC', NOW() FROM teams c WHERE c.name = 'Colonial FC'
AND NOT EXISTS (SELECT 1 FROM teams WHERE name = 'Troy United FC');
