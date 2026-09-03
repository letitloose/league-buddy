-- Per-team control over the automated RSVP reminder cascade — previously a
-- single hardcoded global schedule (3/2/1 days out, fired once daily at
-- 9am Eastern for every team alike). Defaults match that prior global
-- behavior exactly, so no existing team's reminders change unless its
-- captain edits these settings.
ALTER TABLE teams
    ADD COLUMN remindersEnabled BOOLEAN NOT NULL DEFAULT 1,
    ADD COLUMN reminderDaysOut INTEGER NOT NULL DEFAULT 3,
    ADD COLUMN reminderTime TIME NOT NULL DEFAULT '09:00:00';
