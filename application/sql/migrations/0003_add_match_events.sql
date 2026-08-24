CREATE TABLE matchGoals (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    matchID INTEGER NOT NULL,
    teamID INTEGER NOT NULL,
    scorerPlayerID INTEGER NULL,
    assisterPlayerID INTEGER NULL
);
ALTER TABLE matchGoals ADD CONSTRAINT fk_matchgoals_match FOREIGN KEY (matchID) REFERENCES matches(id);
ALTER TABLE matchGoals ADD CONSTRAINT fk_matchgoals_team FOREIGN KEY (teamID) REFERENCES teams(id);
ALTER TABLE matchGoals ADD CONSTRAINT fk_matchgoals_scorer FOREIGN KEY (scorerPlayerID) REFERENCES players(id);
ALTER TABLE matchGoals ADD CONSTRAINT fk_matchgoals_assister FOREIGN KEY (assisterPlayerID) REFERENCES players(id);
CREATE INDEX matchgoals_match_idx ON matchGoals (matchID);

CREATE TABLE matchCards (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    matchID INTEGER NOT NULL,
    teamID INTEGER NOT NULL,
    playerID INTEGER NULL,
    cardType VARCHAR(10) NOT NULL
);
ALTER TABLE matchCards ADD CONSTRAINT fk_matchcards_match FOREIGN KEY (matchID) REFERENCES matches(id);
ALTER TABLE matchCards ADD CONSTRAINT fk_matchcards_team FOREIGN KEY (teamID) REFERENCES teams(id);
ALTER TABLE matchCards ADD CONSTRAINT fk_matchcards_player FOREIGN KEY (playerID) REFERENCES players(id);
CREATE INDEX matchcards_match_idx ON matchCards (matchID);
