-- Per-test data reset. Truncates user-data tables; leaves the `migration`
-- table untouched (migrations are applied once per test process, not per
-- test, in testHelpers.go).
SET FOREIGN_KEY_CHECKS = 0;
TRUNCATE TABLE rsvps;
TRUNCATE TABLE matchTeamNotes;
TRUNCATE TABLE matchGoals;
TRUNCATE TABLE matchCards;
TRUNCATE TABLE playerMatchStats;
TRUNCATE TABLE matches;
TRUNCATE TABLE seasons;
TRUNCATE TABLE teamJoinRequests;
TRUNCATE TABLE teamMembers;
TRUNCATE TABLE leagueAdmins;
TRUNCATE TABLE teamScorekeepers;
TRUNCATE TABLE invites;
TRUNCATE TABLE auditLog;
TRUNCATE TABLE sessions;
TRUNCATE TABLE userRole;
TRUNCATE TABLE roles;
TRUNCATE TABLE players;
TRUNCATE TABLE locations;
TRUNCATE TABLE address;
TRUNCATE TABLE users;
TRUNCATE TABLE teams;
TRUNCATE TABLE leagues;
SET FOREIGN_KEY_CHECKS = 1;

INSERT INTO roles (code, display) VALUES ('ADMIN', 'Administrator');
INSERT INTO leagues (id, name, created) VALUES (1, 'Test League', UTC_TIMESTAMP());
INSERT INTO teams (id, leagueID, name, created) VALUES (1, 1, 'Test Team', UTC_TIMESTAMP());
