package services

import (
	"database/sql"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
)

type CommonService struct {
	DB *sql.DB
}

func (service *CommonService) InsertAuditLog(email string, changeTime time.Time, description string) error {
	am := &models.AuditLogModel{DB: service.DB}

	logEntry := &models.AuditLog{UserEmail: email, ChangeDate: changeTime, Description: description}
	err := am.Insert(logEntry)

	return err
}
