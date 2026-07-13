package models

import (
	"testing"
	"time"
)

func TestInsertAuditLog(t *testing.T) {
	db := NewTestDB(t)

	am := AuditLogModel{DB: db}

	entry := &AuditLog{UserEmail: "lou@example.com", ChangeDate: time.Now(), Description: "did a thing"}
	if err := am.Insert(entry); err != nil {
		t.Fatal(err)
	}

	results, err := am.SearchByDescription("did a thing")
	if err != nil {
		t.Fatal(err)
	}

	expected := 1
	if len(results) != expected {
		t.Fatalf("wrong number of results. expecting %d, got %d", expected, len(results))
	}
}
