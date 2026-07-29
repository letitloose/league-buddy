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

-- Top-level scope entity: a league contains one or more teams. motto and
-- establishedDate are optional display fields, separate from the internal
-- "created" audit timestamp below.
CREATE TABLE leagues (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(255) NOT NULL,
    motto VARCHAR(255) NULL,
    establishedDate DATE NULL,
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
    motto VARCHAR(255) NULL,
    establishedDate DATE NULL,
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

-- A physical field/venue a team can call its home field. Address is
-- required (a location without one isn't useful) and reuses the same
-- `address` table players use. addressKey is a normalized (lowercased,
-- trimmed) concatenation of the address fields, computed in Go — the
-- uniqueness constraint on it is what lets team managers add a "new"
-- location that happens to match an existing one and transparently get
-- routed to the existing row instead of creating a duplicate.
CREATE TABLE locations (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(255) NOT NULL,
    addressID INTEGER NOT NULL,
    addressKey VARCHAR(512) NOT NULL,
    created DATETIME NOT NULL
);
ALTER TABLE locations ADD CONSTRAINT fk_locations_address FOREIGN KEY (addressID) REFERENCES address(id);
ALTER TABLE locations ADD CONSTRAINT uq_locations_addresskey UNIQUE (addressKey);

-- A team's home field is optional and set independently of its league,
-- same forward-reference pattern as captainPlayerID below.
ALTER TABLE teams ADD COLUMN locationID INTEGER NULL;
ALTER TABLE teams ADD CONSTRAINT fk_teams_location FOREIGN KEY (locationID) REFERENCES locations(id);

-- Team membership lives in teamMembers now (see below) — a self-registered
-- user's placeholder player starts unaffiliated (zero memberships) until an
-- invite (at signup) or an approved join request adds one.
CREATE TABLE players (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    firstname VARCHAR(100) NOT NULL,
    lastname VARCHAR(100) NOT NULL,
    dateOfBirth DATE,
    addressID INTEGER,
    email VARCHAR(255) UNIQUE,
    phonenumber VARCHAR(40),
    created DATETIME NOT NULL
);
ALTER TABLE players ADD CONSTRAINT fk_players_address FOREIGN KEY (addressID) REFERENCES address(id);

ALTER TABLE teams ADD CONSTRAINT fk_teams_captain FOREIGN KEY (captainPlayerID) REFERENCES players(id);
ALTER TABLE teams ADD CONSTRAINT uq_teams_captain UNIQUE (captainPlayerID);

-- A player can be a member of many teams, but the service layer enforces at
-- most one per league (checked via a JOIN to teams.leagueID here, not a DB
-- constraint — leagueID isn't denormalized onto this row since a team's
-- league can change after the fact via admin team-update).
CREATE TABLE teamMembers (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    playerID INTEGER NOT NULL,
    teamID INTEGER NOT NULL,
    joinedAt DATETIME NOT NULL,
    CONSTRAINT uq_teammembers_player_team UNIQUE (playerID, teamID)
);
ALTER TABLE teamMembers ADD CONSTRAINT fk_teammembers_player FOREIGN KEY (playerID) REFERENCES players(id);
ALTER TABLE teamMembers ADD CONSTRAINT fk_teammembers_team FOREIGN KEY (teamID) REFERENCES teams(id);
CREATE INDEX teammembers_team_idx ON teamMembers (teamID);
CREATE INDEX teammembers_player_idx ON teamMembers (playerID);

-- A player can administer many leagues, and a league can have many admins —
-- league admins can CRUD teams and players within their league(s), a tier
-- below system admin (everything) and above team captain (one team only).
CREATE TABLE leagueAdmins (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    playerID INTEGER NOT NULL,
    leagueID INTEGER NOT NULL,
    createdAt DATETIME NOT NULL,
    CONSTRAINT uq_leagueadmins_player_league UNIQUE (playerID, leagueID)
);
ALTER TABLE leagueAdmins ADD CONSTRAINT fk_leagueadmins_player FOREIGN KEY (playerID) REFERENCES players(id);
ALTER TABLE leagueAdmins ADD CONSTRAINT fk_leagueadmins_league FOREIGN KEY (leagueID) REFERENCES leagues(id);
CREATE INDEX leagueadmins_league_idx ON leagueAdmins (leagueID);
CREATE INDEX leagueadmins_player_idx ON leagueAdmins (playerID);

-- A league-scoped block of time (e.g. "Spring 2024") containing a
-- round-robin of matches. startDate/endDate are optional and used only to
-- pick a sensible "current" season for a league (see SeasonModel.GetCurrent)
-- when several exist.
CREATE TABLE seasons (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    leagueID INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    startDate DATE NULL,
    endDate DATE NULL,
    created DATETIME NOT NULL
);
ALTER TABLE seasons ADD CONSTRAINT fk_seasons_league FOREIGN KEY (leagueID) REFERENCES leagues(id);
CREATE INDEX seasons_league_idx ON seasons (leagueID);

-- A single game within a season. homeScore/awayScore are nullable — a
-- scheduled-but-not-yet-played (or historically unrecorded) match has no
-- score. notes covers free-text asides (e.g. a short-handed opponent) and,
-- for historical rows where only a win/loss/draw outcome is known but not
-- the exact score, a human-readable stand-in for the missing number.
CREATE TABLE matches (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    seasonID INTEGER NOT NULL,
    homeTeamID INTEGER NOT NULL,
    awayTeamID INTEGER NOT NULL,
    matchDate DATE NOT NULL,
    locationID INTEGER NULL,
    homeScore INTEGER NULL,
    awayScore INTEGER NULL,
    notes VARCHAR(255) NULL,
    created DATETIME NOT NULL
);
ALTER TABLE matches ADD CONSTRAINT fk_matches_season FOREIGN KEY (seasonID) REFERENCES seasons(id);
ALTER TABLE matches ADD CONSTRAINT fk_matches_hometeam FOREIGN KEY (homeTeamID) REFERENCES teams(id);
ALTER TABLE matches ADD CONSTRAINT fk_matches_awayteam FOREIGN KEY (awayTeamID) REFERENCES teams(id);
ALTER TABLE matches ADD CONSTRAINT fk_matches_location FOREIGN KEY (locationID) REFERENCES locations(id);
CREATE INDEX matches_season_idx ON matches (seasonID);
CREATE INDEX matches_hometeam_idx ON matches (homeTeamID);
CREATE INDEX matches_awayteam_idx ON matches (awayTeamID);
CREATE INDEX matches_date_idx ON matches (matchDate);

-- One player's stat line for one match. teamID is denormalized (not derived
-- via a join to matches) since a player's team can differ season to
-- season — this records which team they were on for that specific match.
CREATE TABLE playerMatchStats (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    matchID INTEGER NOT NULL,
    playerID INTEGER NOT NULL,
    teamID INTEGER NOT NULL,
    goals INTEGER NOT NULL DEFAULT 0,
    assists INTEGER NOT NULL DEFAULT 0,
    yellowCards INTEGER NOT NULL DEFAULT 0,
    redCards INTEGER NOT NULL DEFAULT 0,
    CONSTRAINT uq_pms_match_player UNIQUE (matchID, playerID)
);
ALTER TABLE playerMatchStats ADD CONSTRAINT fk_pms_match FOREIGN KEY (matchID) REFERENCES matches(id);
ALTER TABLE playerMatchStats ADD CONSTRAINT fk_pms_player FOREIGN KEY (playerID) REFERENCES players(id);
ALTER TABLE playerMatchStats ADD CONSTRAINT fk_pms_team FOREIGN KEY (teamID) REFERENCES teams(id);
CREATE INDEX pms_player_idx ON playerMatchStats (playerID);
CREATE INDEX pms_team_idx ON playerMatchStats (teamID);

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

-- An active player's request to join a specific team. At most one PENDING
-- row per player *per league* at a time (enforced in the service layer, not
-- a DB constraint, same as the one-team-per-league membership rule) —
-- approving adds a teamMembers row and auto-rejects any other pending
-- request by that player in the same league.
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

INSERT INTO leagues (name, created) VALUES ('CapReg over 30', UTC_TIMESTAMP());
INSERT INTO teams (leagueID, name, motto, created) VALUES (1, 'Colonial FC', 'just an animal looking for a home', UTC_TIMESTAMP());
INSERT INTO roles (code, display) VALUES ('ADMIN', 'Administrator');

INSERT INTO address (address1, city, stateProvince) VALUES ('100 Phillips Rd', 'East Greenbush', 'NY');
INSERT INTO locations (name, addressID, addressKey, created) VALUES ('East Greenbush Soccer Club', LAST_INSERT_ID(), '100 phillips rd||east greenbush|ny|', UTC_TIMESTAMP());
UPDATE teams SET locationID = LAST_INSERT_ID() WHERE id = 1;
