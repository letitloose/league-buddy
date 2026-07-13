package models

import (
	"database/sql"
	"time"
)

type AuditLog struct {
	UserEmail   string
	ChangeDate  time.Time
	Description string
}

type AuditLogModel struct {
	DB *sql.DB
}

func (m *AuditLogModel) Insert(auditLog *AuditLog) error {

	statement := `INSERT INTO auditLog (userEmail, changeDate, description) VALUES(?, ?, ?)`

	_, err := m.DB.Exec(statement, auditLog.UserEmail, auditLog.ChangeDate, auditLog.Description)

	return err
}

func (m *AuditLogModel) SearchByDescription(description string) ([]*AuditLog, error) {

	wildCardDesc := "%" + description + "%"
	statement := `select userEmail, changeDate, description
		from auditLog
		where  description LIKE ?
		order by changeDate DESC;`

	rows, err := m.DB.Query(statement, wildCardDesc)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []*AuditLog{}
	for rows.Next() {
		entry := AuditLog{}
		err := rows.Scan(&entry.UserEmail, &entry.ChangeDate, &entry.Description)
		if err != nil {
			return nil, err
		}

		entries = append(entries, &entry)
	}

	return entries, err
}
