-- Lets a captain permanently dismiss the "New captains start here!" home
-- page banner (see PlayerModel.DismissCaptainGuideBanner) once they're
-- comfortable with the site — NULL (the default) means still shown.
ALTER TABLE players ADD COLUMN captainGuideDismissedAt DATETIME NULL;
