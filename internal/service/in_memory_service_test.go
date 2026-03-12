package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestInMemoryProcessFile(t *testing.T) {
	svc := NewInMemoryImportService()

	// create temp CSV
	tmp := filepath.Join(os.TempDir(), "test_import_"+uuid.New().String()+".csv")
	f, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp)
	f.WriteString("code,discount_type,discount_value,expires_at,max_uses\n")
	f.WriteString("GOOD1,percentage,10,2099-01-01T00:00:00Z,1\n")
	f.WriteString("BAD,invalid,10,2099-01-01T00:00:00Z,1\n")
	f.WriteString("GOOD2,fixed,5,2099-01-01T00:00:00Z,0\n")
	f.Close()

	job, err := svc.CreateJob("test.csv")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.ProcessFile(job.ID, tmp); err != nil {
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
