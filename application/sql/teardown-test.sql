-- Same as teardown.sql — drops all application tables in the current
-- database (league_buddy_test, selected via the test DSN) so setup.sql can
-- recreate them from scratch at the start of a test run.
SET FOREIGN_KEY_CHECKS = 0;
DROP TABLE IF EXISTS rsvps;
DROP TABLE IF EXISTS playerMatchStats;
DROP TABLE IF EXISTS matches;
DROP TABLE IF EXISTS seasons;
DROP TABLE IF EXISTS teamJoinRequests;
DROP TABLE IF EXISTS teamMembers;
DROP TABLE IF EXISTS leagueAdmins;
DROP TABLE IF EXISTS invites;
DROP TABLE IF EXISTS auditLog;
DROP TABLE IF EXISTS userRole;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS players;
DROP TABLE IF EXISTS locations;
DROP TABLE IF EXISTS address;
DROP TABLE IF EXISTS teams;
DROP TABLE IF EXISTS leagues;
DROP TABLE IF EXISTS migration;
DROP TABLE IF EXISTS sessions;
SET FOREIGN_KEY_CHECKS = 1;
