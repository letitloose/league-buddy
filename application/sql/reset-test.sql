-- Per-test data reset. Truncates user-data tables; leaves the `migration`
-- table untouched (migrations are applied once per test process, not per
-- test, in testHelpers.go).
SET FOREIGN_KEY_CHECKS = 0;
TRUNCATE TABLE auditLog;
TRUNCATE TABLE sessions;
TRUNCATE TABLE userRole;
TRUNCATE TABLE roles;
TRUNCATE TABLE players;
TRUNCATE TABLE address;
TRUNCATE TABLE users;
TRUNCATE TABLE teams;
SET FOREIGN_KEY_CHECKS = 1;

INSERT INTO roles (code, display) VALUES ('ADMIN', 'Administrator');
INSERT INTO teams (id, name, created) VALUES (1, 'Test Team', UTC_TIMESTAMP());
