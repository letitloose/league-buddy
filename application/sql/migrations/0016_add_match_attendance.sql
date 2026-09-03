-- Records an explicit attendance override for one player in one match —
-- the default (no row here) is "derived from their RSVP": RSVP'd yes means
-- attended, anything else means didn't. A row here always wins over that
-- default, letting a captain add a walk-on who never RSVP'd or mark a
-- no-show who'd RSVP'd yes. Used to compute the roster's MP (matches
-- played) stat.
CREATE TABLE matchAttendance (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    matchID INTEGER NOT NULL,
    playerID INTEGER NOT NULL,
    teamID INTEGER NOT NULL,
    attended BOOLEAN NOT NULL,
    updatedAt DATETIME NOT NULL,
    CONSTRAINT uq_matchattendance_match_player UNIQUE (matchID, playerID)
);
ALTER TABLE matchAttendance ADD CONSTRAINT fk_matchattendance_match FOREIGN KEY (matchID) REFERENCES matches(id);
ALTER TABLE matchAttendance ADD CONSTRAINT fk_matchattendance_player FOREIGN KEY (playerID) REFERENCES players(id);
ALTER TABLE matchAttendance ADD CONSTRAINT fk_matchattendance_team FOREIGN KEY (teamID) REFERENCES teams(id);
CREATE INDEX matchattendance_match_idx ON matchAttendance (matchID);
CREATE INDEX matchattendance_player_idx ON matchAttendance (playerID);
