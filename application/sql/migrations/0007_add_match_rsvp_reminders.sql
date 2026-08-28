CREATE TABLE matchRSVPReminders (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    matchID INTEGER NOT NULL,
    playerID INTEGER NOT NULL,
    daysOut INTEGER NOT NULL,
    sentAt DATETIME NOT NULL,
    CONSTRAINT uq_matchrsvpreminders_match_player_days UNIQUE (matchID, playerID, daysOut)
);
ALTER TABLE matchRSVPReminders ADD CONSTRAINT fk_matchrsvpreminders_match FOREIGN KEY (matchID) REFERENCES matches(id);
ALTER TABLE matchRSVPReminders ADD CONSTRAINT fk_matchrsvpreminders_player FOREIGN KEY (playerID) REFERENCES players(id);
CREATE INDEX matchrsvpreminders_match_idx ON matchRSVPReminders (matchID);
