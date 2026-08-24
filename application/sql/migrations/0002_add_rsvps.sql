CREATE TABLE rsvps (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    matchID INTEGER NOT NULL,
    playerID INTEGER NOT NULL,
    teamID INTEGER NOT NULL,
    status VARCHAR(10) NOT NULL,
    message VARCHAR(255) NULL,
    respondedAt DATETIME NOT NULL,
    CONSTRAINT uq_rsvps_match_player UNIQUE (matchID, playerID)
);
ALTER TABLE rsvps ADD CONSTRAINT fk_rsvps_match FOREIGN KEY (matchID) REFERENCES matches(id);
ALTER TABLE rsvps ADD CONSTRAINT fk_rsvps_player FOREIGN KEY (playerID) REFERENCES players(id);
ALTER TABLE rsvps ADD CONSTRAINT fk_rsvps_team FOREIGN KEY (teamID) REFERENCES teams(id);
CREATE INDEX rsvps_match_idx ON rsvps (matchID);
CREATE INDEX rsvps_player_idx ON rsvps (playerID);
