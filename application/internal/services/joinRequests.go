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

// RequestToJoin lets an unaffiliated active player ask to join teamID. Fails
// if the player already has a team, or already has a pending request.
func (service *JoinRequestService) RequestToJoin(playerID, teamID int, actorEmail string) error {
	pm := &models.PlayerModel{DB: service.DB}
	player, err := pm.Get(playerID)
	if err != nil {
		return err
	}
	if player.TeamID.Valid {
		return models.ErrBadData
	}

	if _, err := service.GetPendingByPlayer(playerID); err == nil {
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

	pm := &models.PlayerModel{DB: service.DB}
	if err := pm.SetTeam(jr.PlayerID, jr.TeamID); err != nil {
		return err
	}

	if err := service.UpdateStatus(requestID, "APPROVED", respondedByUserID); err != nil {
		return err
	}
	if err := service.RejectOtherPending(jr.PlayerID, requestID, respondedByUserID); err != nil {
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
