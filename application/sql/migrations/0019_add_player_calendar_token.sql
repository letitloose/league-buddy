-- Lets a player subscribe to their personal match schedule from their
-- phone's calendar app via a secret, regenerable token embedded in a
-- public ICS feed URL (see PlayerModel.GetByCalendarToken) — nullable and
-- lazily generated the first time a player visits their Notification
-- Preferences page (see CalendarService.EnsureToken), so every existing
-- player is unaffected until they opt in. MariaDB permits multiple NULLs
-- under a UNIQUE constraint, so this is safe pre-backfill.
ALTER TABLE players ADD COLUMN calendarToken CHAR(64) NULL;
ALTER TABLE players ADD CONSTRAINT uq_players_calendartoken UNIQUE (calendarToken);
