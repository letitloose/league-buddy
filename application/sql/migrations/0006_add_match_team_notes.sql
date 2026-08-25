CREATE TABLE matchTeamNotes (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    matchID INTEGER NOT NULL,
    teamID INTEGER NOT NULL,
    playerOfMatchID INTEGER NULL,
    notes TEXT NULL,
    updatedAt DATETIME NOT NULL,
    CONSTRAINT uq_matchteamnotes_match_team UNIQUE (matchID, teamID)
);
ALTER TABLE matchTeamNotes ADD CONSTRAINT fk_matchteamnotes_match FOREIGN KEY (matchID) REFERENCES matches(id);
ALTER TABLE matchTeamNotes ADD CONSTRAINT fk_matchteamnotes_team FOREIGN KEY (teamID) REFERENCES teams(id);
ALTER TABLE matchTeamNotes ADD CONSTRAINT fk_matchteamnotes_player FOREIGN KEY (playerOfMatchID) REFERENCES players(id);
CREATE INDEX matchteamnotes_match_idx ON matchTeamNotes (matchID);
