-- A player can be a scorekeeper for many teams, and a team can have many
-- scorekeepers — a captain-designated tier that only grants match-editing
-- rights (score/goals/cards), not full team management (roster, invites).
CREATE TABLE teamScorekeepers (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    playerID INTEGER NOT NULL,
    teamID INTEGER NOT NULL,
    createdAt DATETIME NOT NULL,
    CONSTRAINT uq_teamscorekeepers_player_team UNIQUE (playerID, teamID)
);
ALTER TABLE teamScorekeepers ADD CONSTRAINT fk_teamscorekeepers_player FOREIGN KEY (playerID) REFERENCES players(id);
ALTER TABLE teamScorekeepers ADD CONSTRAINT fk_teamscorekeepers_team FOREIGN KEY (teamID) REFERENCES teams(id);
CREATE INDEX teamscorekeepers_team_idx ON teamScorekeepers (teamID);
CREATE INDEX teamscorekeepers_player_idx ON teamScorekeepers (playerID);
