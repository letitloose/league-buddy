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

-- Top-level scope entity. One row seeded now; more rows + a picker UI is all
-- multi-team/League support needs later — no migration required.
CREATE TABLE teams (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(255) NOT NULL,
    created DATETIME NOT NULL
);

CREATE TABLE address (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    address1 VARCHAR(255) NOT NULL,
    address2 VARCHAR(255),
    city VARCHAR(100) NOT NULL,
    stateProvince VARCHAR(50),
    zipCode VARCHAR(10),
    country VARCHAR(50)
);

-- Primary entity: a team's roster member.
CREATE TABLE players (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    teamID INTEGER NOT NULL,
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

-- Login/auth. playerID links a user to their roster profile (nullable — an
-- admin/coach account need not be a player; a player need not have signed up
-- yet).
CREATE TABLE users (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    email VARCHAR(255) NOT NULL UNIQUE,
    hashed_password CHAR(60) NOT NULL,
    created DATETIME NOT NULL,
    active BOOLEAN NOT NULL DEFAULT 0,
    verification_hash CHAR(60),
    lastlogin DATETIME,
    playerID INTEGER,
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

INSERT INTO teams (name, created) VALUES ('My Team', UTC_TIMESTAMP());
INSERT INTO roles (code, display) VALUES ('ADMIN', 'Administrator');
