-- Per-test data reset. Leaves the `migration` table untouched (migrations
-- are applied once per test process, not per test, in testHelpers.go).
--
-- DELETE, not TRUNCATE: TRUNCATE is DDL (drops/recreates the table under
-- an exclusive metadata lock), which measured at a genuinely fixed ~25-30ms
-- per statement here regardless of how empty the table already was or how
-- durability settings were tuned — with ~28 tables that's ~800ms paid
-- before every single test in this suite. DELETE FROM an empty table (the
-- overwhelmingly common case between tests) is a few milliseconds. Losing
-- TRUNCATE's AUTO_INCREMENT reset is harmless here: nothing in this suite
-- assumes a specific literal auto-generated id, only the two explicit
-- id=1 rows inserted below (an explicit INSERT with FOREIGN_KEY_CHECKS
-- disabled works regardless of where each table's counter currently is).
SET FOREIGN_KEY_CHECKS = 0;
DELETE FROM matchAttendance;
DELETE FROM rsvps;
DELETE FROM matchTeamNotes;
DELETE FROM playerNotificationPreferences;
DELETE FROM matchRSVPReminders;
DELETE FROM matchCaptainMessageReminders;
DELETE FROM matchGoals;
DELETE FROM matchCards;
DELETE FROM playerMatchStats;
DELETE FROM matches;
DELETE FROM seasons;
DELETE FROM teamJoinRequests;
DELETE FROM teamMembers;
DELETE FROM leagueAdmins;
DELETE FROM teamScorekeepers;
DELETE FROM invites;
DELETE FROM auditLog;
DELETE FROM sessions;
DELETE FROM userRole;
DELETE FROM roles;
DELETE FROM players;
DELETE FROM locations;
DELETE FROM address;
DELETE FROM users;
DELETE FROM teams;
DELETE FROM leagues;
SET FOREIGN_KEY_CHECKS = 1;

INSERT INTO roles (code, display) VALUES ('ADMIN', 'Administrator');
INSERT INTO leagues (id, name, created) VALUES (1, 'Test League', UTC_TIMESTAMP());
INSERT INTO teams (id, leagueID, name, created) VALUES (1, 1, 'Test Team', UTC_TIMESTAMP());
