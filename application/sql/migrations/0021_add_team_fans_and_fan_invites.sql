CREATE TABLE teamFans (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    userID INTEGER NOT NULL,
    teamID INTEGER NOT NULL,
    followedAt DATETIME NOT NULL,
    CONSTRAINT uq_teamfans_user_team UNIQUE (userID, teamID)
);
ALTER TABLE teamFans ADD CONSTRAINT fk_teamfans_user FOREIGN KEY (userID) REFERENCES users(id);
ALTER TABLE teamFans ADD CONSTRAINT fk_teamfans_team FOREIGN KEY (teamID) REFERENCES teams(id);
CREATE INDEX teamfans_team_idx ON teamFans (teamID);
CREATE INDEX teamfans_user_idx ON teamFans (userID);

ALTER TABLE invites ADD COLUMN asFan BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN fanCalendarToken VARCHAR(64) NULL;
