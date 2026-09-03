-- Lets a captain move a player between a team's active roster and its
-- "Legends" list (see TeamMemberModel.SetLegendStatus) instead of removing
-- them outright — a Legend stays linked to the team and keeps their career
-- stats, just organized off the active roster. Defaults to 0 (active) so
-- every existing roster member is unaffected.
ALTER TABLE teamMembers ADD COLUMN isLegend BOOLEAN NOT NULL DEFAULT 0;
