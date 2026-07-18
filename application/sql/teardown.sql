-- Drops all application tables in the current database (selected via the
-- connection DSN) so setup.sql can recreate them from scratch. Does not
-- touch the database/schema itself — that's created by the MariaDB
-- container's MYSQL_DATABASE env var.
SET FOREIGN_KEY_CHECKS = 0;
DROP TABLE IF EXISTS teamJoinRequests;
DROP TABLE IF EXISTS invites;
DROP TABLE IF EXISTS auditLog;
DROP TABLE IF EXISTS userRole;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS players;
DROP TABLE IF EXISTS address;
DROP TABLE IF EXISTS teams;
DROP TABLE IF EXISTS leagues;
DROP TABLE IF EXISTS migration;
DROP TABLE IF EXISTS sessions;
SET FOREIGN_KEY_CHECKS = 1;
