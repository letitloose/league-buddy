package services

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"database/sql"

	"github.com/letitloose/league-buddy/internal/models"
)

type JoinRequestService struct {
	*models.JoinRequestModel
	DB *sql.DB
	*Email
	InfoLog *log.Logger
}

// RequestToJoin lets an active player ask to join teamID. Fails if they're
// already a member of teamID, already belong to a different team in the
// same league (a player may hold at most one team per league), or already
// have a pending request in that league.
func (service *JoinRequestService) RequestToJoin(playerID, teamID int, actorEmail string) error {
	tm := &models.TeamModel{DB: service.DB}
	team, err := tm.Get(teamID)
	if err != nil {
		return err
	}

	tmm := &models.TeamMemberModel{DB: service.DB}
	isMember, err := tmm.IsMember(playerID, teamID)
	if err != nil {
		return err
	}
	if isMember {
		return models.ErrBadData
	}

	hasTeamInLeague, err := tmm.HasTeamInLeague(playerID, team.LeagueID)
	if err != nil {
		return err
	}
	if hasTeamInLeague {
		return models.ErrBadData
	}

	if _, err := service.GetPendingByPlayerAndLeague(playerID, team.LeagueID); err == nil {
		return models.ErrDuplicateRequest
	} else if !errors.Is(err, models.ErrNoRecord) {
		return err
	}

	if _, err := service.Insert(&models.TeamJoinRequest{PlayerID: playerID, TeamID: teamID}); err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	if err := cs.InsertAuditLog(actorEmail, time.Now(), "requested to join team"); err != nil {
		return err
	}

	service.notifyCaptain(teamID)

	return nil
}

// notifyCaptain emails (or logs, dev-mode fallback) the team's captain that
// a join request is waiting. If the team has no captain assigned, this is a
// no-op beyond the InfoLog line — the /admin/joinRequests cross-team view is
// what keeps the request from being invisible in that case.
func (service *JoinRequestService) notifyCaptain(teamID int) {
	tm := &models.TeamModel{DB: service.DB}
	team, err := tm.Get(teamID)
	if err != nil {
		if service.InfoLog != nil {
			service.InfoLog.Printf("join request notify: could not load team %d: %v", teamID, err)
		}
		return
	}

	link := fmt.Sprintf("https://%s/team/%d/joinRequests", os.Getenv("VIRTUAL_HOST"), teamID)

	if !team.CaptainPlayerID.Valid {
		if service.InfoLog != nil {
			service.InfoLog.Printf("join request for team %d (%s) has no captain to notify -- see %s", teamID, team.Name, link)
		}
		return
	}

	pm := &models.PlayerModel{DB: service.DB}
	captain, err := pm.Get(int(team.CaptainPlayerID.Int32))
	if err != nil || !captain.Email.Valid {
		if service.InfoLog != nil {
			service.InfoLog.Printf("join request for team %d (%s): captain has no email on file -- see %s", teamID, team.Name, link)
		}
		return
	}

	if service.Email != nil {
		body := fmt.Sprintf(
			`<html>
				<body>
					<p>Someone has requested to join %s. <a href="%s">Review the request here</a>.</p>
				</body>
			</html>`, team.Name, link)
		_ = service.SendEmailV2("New join request for "+team.Name, "", body, captain.Email.String)
	} else if service.InfoLog != nil {
		service.InfoLog.Printf("no email configured -- join request notification for %s (team %d): %s", captain.Email.String, teamID, link)
	}
}

func (service *JoinRequestService) Approve(requestID, respondedByUserID int, actorEmail string) error {
	jr, err := service.Get(requestID)
	if err != nil {
		return err
	}
	if jr.Status != "PENDING" {
		return models.ErrBadData
	}

	tm := &models.TeamModel{DB: service.DB}
	team, err := tm.Get(jr.TeamID)
	if err != nil {
		return err
	}

	tmm := &models.TeamMemberModel{DB: service.DB}
	hasTeamInLeague, err := tmm.HasTeamInLeague(jr.PlayerID, team.LeagueID)
	if err != nil {
		return err
	}
	if hasTeamInLeague {
		// They picked up a team in this league through some other path since
		// requesting (another approval, an invite) — the one-per-league rule
		// still applies at approval time, not just at request time.
		return models.ErrBadData
	}

	if err := tmm.AddMembership(jr.PlayerID, jr.TeamID); err != nil {
		return err
	}

	if err := service.UpdateStatus(requestID, "APPROVED", respondedByUserID); err != nil {
		return err
	}
	if err := service.RejectOtherPending(jr.PlayerID, requestID, respondedByUserID, team.LeagueID); err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), "approved join request")
}

func (service *JoinRequestService) Reject(requestID, respondedByUserID int, actorEmail string) error {
	jr, err := service.Get(requestID)
	if err != nil {
		return err
	}
	if jr.Status != "PENDING" {
		return models.ErrBadData
	}

	if err := service.UpdateStatus(requestID, "REJECTED", respondedByUserID); err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), "rejected join request")
}
