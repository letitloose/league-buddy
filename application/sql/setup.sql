-- Operates on whatever database the connection's DSN already selects
-- (league_buddy in prod, league_buddy_test in tests) — the database itself
-- is created by the MariaDB container's MYSQL_DATABASE env var, not here.
-- Run teardown.sql / teardown-test.sql first if tables already exist.

-- Session store — exact shape required by alexedwards/scs/mysqlstore.
CREATE TABLE sessions (
    token CHAR(43) PRIMARY KEY,
    data BLOB NOT NULL,
    expiry TIMESTAMP(6) NOT NULL
);
CREATE INDEX sessions_expiry_idx ON sessions (expiry);

-- Migration tracking, read by MigrationModel.
CREATE TABLE migration (
    filename VARCHAR(255),
    rundate DATE
);

-- Top-level scope entity: a league contains one or more teams.
CREATE TABLE leagues (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(255) NOT NULL,
    created DATETIME NOT NULL
);

-- A team belongs to exactly one league, and has zero or one captain (a
-- player on its own roster) at a time. captainPlayerID's FK is added below,
-- after players exists (same forward-reference pattern used for
-- players -> teams).
CREATE TABLE teams (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    leagueID INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    captainPlayerID INTEGER NULL,
    created DATETIME NOT NULL
);
ALTER TABLE teams ADD CONSTRAINT fk_teams_league FOREIGN KEY (leagueID) REFERENCES leagues(id);

-- US-only for now; no country column needed at this stage of development.
CREATE TABLE address (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    address1 VARCHAR(255) NOT NULL,
    address2 VARCHAR(255),
    city VARCHAR(100) NOT NULL,
    stateProvince VARCHAR(50),
    zipCode VARCHAR(10)
);

-- teamID is nullable — a self-registered user's placeholder player starts
-- "unaffiliated" until an invite (at signup) or an approved join request
-- assigns a team.
CREATE TABLE players (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    teamID INTEGER NULL,
    firstname VARCHAR(100) NOT NULL,
    lastname VARCHAR(100) NOT NULL,
    dateOfBirth DATE,
    addressID INTEGER,
    email VARCHAR(255) UNIQUE,
    phonenumber VARCHAR(40),
    created DATETIME NOT NULL
);
ALTER TABLE players ADD CONSTRAINT fk_players_team FOREIGN KEY (teamID) REFERENCES teams(id);
ALTER TABLE players ADD CONSTRAINT fk_players_address FOREIGN KEY (addressID) REFERENCES address(id);

ALTER TABLE teams ADD CONSTRAINT fk_teams_captain FOREIGN KEY (captainPlayerID) REFERENCES players(id);
ALTER TABLE teams ADD CONSTRAINT uq_teams_captain UNIQUE (captainPlayerID);

-- Login/auth. playerID links a user to their roster profile (nullable — an
-- admin/coach account need not be a player; a player need not have signed up
-- yet). pendingInviteID is set at signup when a valid invite token was
-- present on the URL, and consumed (cleared) at activation once the
-- invite's team is applied to the linked player.
CREATE TABLE users (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    email VARCHAR(255) NOT NULL UNIQUE,
    hashed_password CHAR(60) NOT NULL,
    created DATETIME NOT NULL,
    active BOOLEAN NOT NULL DEFAULT 0,
    verification_hash CHAR(60),
    lastlogin DATETIME,
    playerID INTEGER,
    pendingInviteID INTEGER NULL,
    CONSTRAINT uq_users_playerID UNIQUE (playerID)
);
ALTER TABLE users ADD CONSTRAINT fk_users_player FOREIGN KEY (playerID) REFERENCES players(id);

-- Kept as real tables so adding a role later (e.g. Coach) is a data change;
-- v1 UI only exposes ADMIN toggling.
CREATE TABLE roles (
    code VARCHAR(20) PRIMARY KEY,
    display VARCHAR(255)
);
CREATE TABLE userRole (
    userID INTEGER,
    roleID VARCHAR(20)
);
ALTER TABLE userRole ADD CONSTRAINT fk_userrole_user FOREIGN KEY (userID) REFERENCES users(id);
ALTER TABLE userRole ADD CONSTRAINT fk_userrole_role FOREIGN KEY (roleID) REFERENCES roles(code);

CREATE TABLE auditLog (
    userEmail VARCHAR(100),
    changeDate DATETIME,
    description VARCHAR(255)
);

-- A single-use signup token a captain/admin emails to a prospective player.
-- email is reference-only (the invited person may register under a
-- different address) — never matched against on activation, only logged.
CREATE TABLE invites (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    token CHAR(64) NOT NULL,
    teamID INTEGER NOT NULL,
    email VARCHAR(255) NOT NULL,
    createdByUserID INTEGER NOT NULL,
    createdAt DATETIME NOT NULL,
    usedAt DATETIME NULL,
    usedByUserID INTEGER NULL,
    CONSTRAINT uq_invites_token UNIQUE (token)
);
ALTER TABLE invites ADD CONSTRAINT fk_invites_team FOREIGN KEY (teamID) REFERENCES teams(id);
ALTER TABLE invites ADD CONSTRAINT fk_invites_createdby FOREIGN KEY (createdByUserID) REFERENCES users(id);
ALTER TABLE invites ADD CONSTRAINT fk_invites_usedby FOREIGN KEY (usedByUserID) REFERENCES users(id);

ALTER TABLE users ADD CONSTRAINT fk_users_pendinginvite FOREIGN KEY (pendingInviteID) REFERENCES invites(id);

-- An unaffiliated active player's request to join a specific team. One
-- PENDING row per player at a time, across any team (enforced in the
-- service layer, not by a DB constraint) — approving sets players.teamID
-- and auto-rejects any other pending request by that player.
CREATE TABLE teamJoinRequests (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    playerID INTEGER NOT NULL,
    teamID INTEGER NOT NULL,
    status VARCHAR(10) NOT NULL DEFAULT 'PENDING',
    requestedAt DATETIME NOT NULL,
    respondedAt DATETIME NULL,
    respondedByUserID INTEGER NULL
);
ALTER TABLE teamJoinRequests ADD CONSTRAINT fk_tjr_player FOREIGN KEY (playerID) REFERENCES players(id);
ALTER TABLE teamJoinRequests ADD CONSTRAINT fk_tjr_team FOREIGN KEY (teamID) REFERENCES teams(id);
ALTER TABLE teamJoinRequests ADD CONSTRAINT fk_tjr_respondedby FOREIGN KEY (respondedByUserID) REFERENCES users(id);
CREATE INDEX tjr_player_status_idx ON teamJoinRequests (playerID, status);
CREATE INDEX tjr_team_status_idx ON teamJoinRequests (teamID, status);

INSERT INTO leagues (name, created) VALUES ('My League', UTC_TIMESTAMP());
INSERT INTO teams (leagueID, name, created) VALUES (1, 'My Team', UTC_TIMESTAMP());
INSERT INTO roles (code, display) VALUES ('ADMIN', 'Administrator');
