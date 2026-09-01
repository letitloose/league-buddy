ALTER TABLE players ADD COLUMN phoneVerifiedAt DATETIME NULL;
ALTER TABLE players ADD COLUMN phoneVerificationCode CHAR(6) NULL;
ALTER TABLE players ADD COLUMN phoneVerificationExpiresAt DATETIME NULL;

CREATE TABLE playerNotificationPreferences (
    playerID INTEGER NOT NULL,
    category VARCHAR(64) NOT NULL,
    channel VARCHAR(16) NOT NULL,
    PRIMARY KEY (playerID, category)
);
ALTER TABLE playerNotificationPreferences ADD CONSTRAINT fk_playernotificationpreferences_player FOREIGN KEY (playerID) REFERENCES players(id);
