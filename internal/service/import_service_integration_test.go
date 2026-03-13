package service

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/glebarez/sqlite"
)

func TestImportServiceWithSQLite(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "testdb-*.db")
	if err != nil {
		t.Fatal(err)
	}
	dbPath := tmpDB.Name()
	tmpDB.Close()
	defer os.Remove(dbPath)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// run migrations
	migrPath := filepath.Join("..", "..", "migrations", "001_init.sql")
	if b, err := os.ReadFile(migrPath); err == nil {
		if _, err := db.Exec(string(b)); err != nil {
			t.Fatalf("migration failed: %v", err)
		}
	} else {
		t.Fatalf("migration file not found: %v", err)
	}

	svc := NewImportService(db)

	// create temp CSV
	tmpCSV, err := os.CreateTemp("", "test-import-*.csv")
	if err != nil {
		t.Fatal(err)
	}
	csvPath := tmpCSV.Name()
	tmpCSV.WriteString("code,discount_type,discount_value,expires_at,max_uses\n")
	tmpCSV.WriteString("GOOD1,percentage,10,2099-01-01T00:00:00Z,1\n")
	tmpCSV.WriteString("BAD,invalid,10,2099-01-01T00:00:00Z,1\n")
	tmpCSV.WriteString("GOOD2,fixed,5,2099-01-01T00:00:00Z,0\n")
	tmpCSV.Close()
	defer os.Remove(csvPath)

	job, err := svc.CreateJob("integration.csv")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.ProcessFile(job.ID, csvPath); err != nil {
		t.Fatal(err)
	}

	j, err := svc.GetJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if j.TotalRows != 3 {
		t.Fatalf("expected 3 total rows, got %d", j.TotalRows)
	}
	if j.SuccessCount != 2 {
		t.Fatalf("expected 2 successes, got %d", j.SuccessCount)
	}
	if j.FailureCount != 1 {
		t.Fatalf("expected 1 failure, got %d", j.FailureCount)
	}
}
