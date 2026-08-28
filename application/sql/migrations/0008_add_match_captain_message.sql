ALTER TABLE matchTeamNotes ADD COLUMN captainMessage TEXT NULL;

CREATE TABLE matchCaptainMessageReminders (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    matchID INTEGER NOT NULL,
    teamID INTEGER NOT NULL,
    sentAt DATETIME NOT NULL,
    CONSTRAINT uq_matchcaptainmessagereminders_match_team UNIQUE (matchID, teamID)
);
ALTER TABLE matchCaptainMessageReminders ADD CONSTRAINT fk_matchcaptainmessagereminders_match FOREIGN KEY (matchID) REFERENCES matches(id);
ALTER TABLE matchCaptainMessageReminders ADD CONSTRAINT fk_matchcaptainmessagereminders_team FOREIGN KEY (teamID) REFERENCES teams(id);
